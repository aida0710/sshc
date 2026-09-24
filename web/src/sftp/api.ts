import { ApiError, apiClient } from "../api/client";
import { issueAction, patchJSON, postJSON, putJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { saveWithAndroid } from "../android/native";
import { vpnProblemCodes } from "../vpn/vpnRefusals";

export type RemoteEntry = components["schemas"]["SFTPEntry"];
export type LocalListing = components["schemas"]["SFTPLocalListing"];
export type RemoteTextFile = components["schemas"]["SFTPTextFile"];
export type ResumableUpload = components["schemas"]["SFTPResumableUpload"];
export type TransferJob = components["schemas"]["SFTPTransferJob"];
type CreateTransferJobWire = components["schemas"]["SFTPCreateTransferJobRequest"];
export type CreateTransferJob = Omit<CreateTransferJobWire, "sourceAlias" | "sourcePath" | "operation" | "overwrite"> &
  Partial<Pick<CreateTransferJobWire, "sourceAlias" | "sourcePath" | "operation" | "overwrite">>;
export type TransferDirection = TransferJob["direction"];
export type TransferKind = TransferJob["kind"];
export type TransferJobStatus = TransferJob["status"];
export type TransferJobAction = components["schemas"]["SFTPTransferJobActionRequest"]["action"];
export type TransferControlAction = TransferJob["allowedActions"][number];
export type TransferQueueMove = components["schemas"]["SFTPTransferQueueMoveRequest"]["move"];
export type TransferSettings = components["schemas"]["SFTPTransferSettingsRequest"];
export type TransferJobList = components["schemas"]["SFTPTransferJobList"];

export type RemoteSearchResult = components["schemas"]["SFTPSearchResult"];
export type DirectoryComparison = components["schemas"]["SFTPDirectoryComparison"];
export type RemoteDirectoryStats = components["schemas"]["SFTPDirectoryStats"];

export type RemotePreview = {
  contentType: string;
  blob: Blob;
  revision: string;
};

// preview が返しうる問題は、詳細モーダルがその場で言葉にする。共通の失敗
// 通知まで重ねると、preview できない普通のファイルを開くたびに全画面の
// 警告が出る。
const previewProblems = [
  "sftp_preview_type",
  "sftp_preview_too_large",
  "sftp_not_found",
  "sftp_wrong_type",
  "sftp_conflict",
  "sftp_failed",
];

// Background transfer failures are rendered by TransferManagerList and its
// completion notice. Reporting the same expected failure as an application-wide
// diagnostic obscures the queue controls, especially on a narrow screen.
const transferProblems = [
  "sftp_failed",
  "sftp_cleanup_pending",
  "sftp_conflict",
  "sftp_exists",
  "sftp_not_found",
  "sftp_range_invalid",
  "sftp_transfer_limit",
  "sftp_transfer_not_found",
  "sftp_transfer_state",
  "sftp_transfer_too_large",
  "sftp_unsupported_entry",
];

export type StreamDownloadOptions = {
  signal?: AbortSignal;
  revision?: string;
  onChunk: (chunk: Uint8Array, total: number | null) => void | Promise<void>;
  onRevision?: (revision: string) => void;
  onReset?: (total: number | null) => void | Promise<void>;
};

function pathFor(alias: string, suffix: string, remotePath: string): string {
  return `/api/v1/sftp/${encodeURIComponent(alias)}/${suffix}?path=${encodeURIComponent(remotePath)}`;
}

function entry(value: unknown): RemoteEntry {
  return validateOpenAPISchema<RemoteEntry>("SFTPEntry", value);
}

function resumableUpload(value: unknown): ResumableUpload {
  return validateOpenAPISchema<ResumableUpload>("SFTPResumableUpload", value);
}

function transferJob(value: unknown): TransferJob {
  return validateOpenAPISchema<TransferJob>("SFTPTransferJob", value);
}

export const sftpApi = {
  async listLocal(path = ""): Promise<LocalListing> {
    const query = path === "" ? "" : `?path=${encodeURIComponent(path)}`;
    return validateOpenAPISchema<LocalListing>("SFTPLocalListing", await apiClient.read(`/api/v1/sftp/local/entries${query}`));
  },
  async listTransfers(): Promise<TransferJobList> {
    return validateOpenAPISchema<TransferJobList>("SFTPTransferJobList", await apiClient.read("/api/v1/sftp/transfers"));
  },
  async updateTransferSettings(settings: TransferSettings): Promise<TransferJobList> {
    return validateOpenAPISchema<TransferJobList>("SFTPTransferJobList", await putJSON<unknown>("/api/v1/sftp/transfers/settings", settings, transferProblems));
  },
  async moveTransfer(id: string, move: TransferQueueMove): Promise<TransferJobList> {
    return validateOpenAPISchema<TransferJobList>("SFTPTransferJobList", await postJSON<unknown>(`/api/v1/sftp/transfers/${encodeURIComponent(id)}/queue-position`, { move }, undefined, transferProblems));
  },
  async createTransfer(input: CreateTransferJob): Promise<TransferJob> {
    const actionToken = input.direction === "remote" && input.operation === "delete"
      ? await issueAction("sftp.delete", `${input.alias}:${input.remotePath}`)
      : null;
    return transferJob(await postJSON<unknown>(
      "/api/v1/sftp/transfers",
      { sourceAlias: "", sourcePath: "", operation: "", overwrite: false, ...input },
      actionToken ?? undefined,
    ));
  },
  async compareDirectories(leftAlias: string, leftPath: string, rightAlias: string, rightPath: string): Promise<DirectoryComparison> {
    const query = new URLSearchParams({ leftAlias, leftPath, rightAlias, rightPath });
    return validateOpenAPISchema<DirectoryComparison>("SFTPDirectoryComparison", await apiClient.read(`/api/v1/sftp/compare?${query.toString()}`, {
      locallyHandledCodes: ["sftp_failed", "sftp_compare_limit", "sftp_not_found"],
    }));
  },
  async clearFinishedTransfers(): Promise<void> {
    await apiClient.mutate<unknown>("/api/v1/sftp/transfers/finished", { method: "DELETE" }, { locallyHandledCodes: transferProblems });
  },
  async removeTransfer(id: string): Promise<void> {
    await apiClient.mutate<unknown>(`/api/v1/sftp/transfers/${encodeURIComponent(id)}`, { method: "DELETE" }, {
      locallyHandledCodes: transferProblems,
    });
  },
  async updateTransfer(id: string, action: TransferJobAction, options: { transferredBytes?: number; totalBytes?: number; problem?: string; resetProgress?: boolean } = {}): Promise<TransferJob> {
    return transferJob(await postJSON<unknown>(`/api/v1/sftp/transfers/${encodeURIComponent(id)}/actions`, { action, ...options }, undefined, transferProblems));
  },
  async checkpointDownload(id: string, offset: number, revision: string): Promise<TransferJob> {
    return transferJob(await postJSON<unknown>(`/api/v1/sftp/transfers/${encodeURIComponent(id)}/download-checkpoint`, { offset, revision }, undefined, transferProblems));
  },
  async verifyDownload(alias: string, jobId: string, remotePath: string, revision: string): Promise<void> {
    const endpoint = `${pathFor(alias, "download", remotePath)}&jobId=${encodeURIComponent(jobId)}&verify=true`;
    const response = await apiClient.send(endpoint, { method: "GET", headers: { "If-Range": revision } });
    if (response.status !== 204) throw new Error("download_changed");
  },
  async list(alias: string, remotePath = ""): Promise<{ path: string; entries: RemoteEntry[] }> {
    // Directory listing also establishes the SFTP connection. An unavailable host, or a
    // VPN route that could not reach it, is an expected result handled inline by
    // SFTPPanel, not an application-wide failure.
    const endpoint = remotePath === ""
      ? `/api/v1/sftp/${encodeURIComponent(alias)}/entries`
      : pathFor(alias, "entries", remotePath);
    return validateOpenAPISchema<components["schemas"]["SFTPListing"]>("SFTPListing", await apiClient.read(endpoint, {
      locallyHandledCodes: ["sftp_failed", ...vpnProblemCodes],
    }));
  },
  async search(alias: string, remotePath: string, query: string): Promise<RemoteSearchResult> {
    const endpoint = `/api/v1/sftp/${encodeURIComponent(alias)}/search?path=${encodeURIComponent(remotePath)}&query=${encodeURIComponent(query)}`;
    return validateOpenAPISchema<RemoteSearchResult>("SFTPSearchResult", await apiClient.read(endpoint, {
      locallyHandledCodes: ["sftp_failed", "sftp_not_found", "invalid_request"],
    }));
  },
  async directoryStats(alias: string, remotePath: string): Promise<RemoteDirectoryStats> {
    return validateOpenAPISchema<RemoteDirectoryStats>("SFTPDirectoryStats", await apiClient.read(pathFor(alias, "stats", remotePath), {
      locallyHandledCodes: ["sftp_failed", "sftp_not_found", "sftp_wrong_type"],
    }));
  },
  async previewFile(alias: string, remotePath: string): Promise<RemotePreview> {
    const endpoint = pathFor(alias, "preview", remotePath);
    const response = await apiClient.send(endpoint, { method: "GET" }, { locallyHandledCodes: previewProblems });
    if (!response.ok) {
      const problem = await response.json().catch(() => null) as { code?: unknown } | null;
      const code = typeof problem?.code === "string" ? problem.code : "sftp_failed";
      throw new ApiError(code, response.status, null);
    }
    return {
      contentType: (response.headers.get("Content-Type") ?? "").split(";")[0]?.trim() ?? "",
      revision: response.headers.get("ETag") ?? "",
      blob: await response.blob(),
    };
  },
  async readText(alias: string, remotePath: string): Promise<RemoteTextFile> {
    return validateOpenAPISchema<RemoteTextFile>("SFTPTextFile", await apiClient.read(pathFor(alias, "text", remotePath)));
  },
  async saveText(alias: string, remotePath: string, contents: string, expectedRevision: string): Promise<RemoteTextFile> {
    return validateOpenAPISchema<RemoteTextFile>("SFTPTextFile", await putJSON<unknown>(pathFor(alias, "text", remotePath), { contents, expectedRevision }));
  },
  async mkdir(alias: string, remotePath: string): Promise<RemoteEntry> {
    return entry(await postJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/entries`, { path: remotePath, type: "directory" }));
  },
  async createEmptyFile(alias: string, remotePath: string): Promise<RemoteEntry> {
    return entry(await postJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/entries`, { path: remotePath, type: "file" }, undefined, ["sftp_exists", "sftp_failed"]));
  },
  async rename(alias: string, from: string, to: string): Promise<RemoteEntry> {
    return entry(await patchJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/entry`, { from, to }));
  },
  async remove(alias: string, remotePath: string): Promise<void> {
    const target = `${alias}:${remotePath}`;
    const token = await issueAction("sftp.delete", target);
    await apiClient.mutate<unknown>(pathFor(alias, "entry", remotePath), {
      method: "DELETE",
      headers: { "X-SSHC-Action": token },
    });
  },
  async startUpload(alias: string, id: string, remotePath: string, size: number, sourceFingerprint: string): Promise<ResumableUpload> {
    return resumableUpload(await postJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}`, { path: remotePath, size, sourceFingerprint }, undefined, transferProblems));
  },
  async appendUpload(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload> {
    const query = `/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}?path=${encodeURIComponent(remotePath)}&offset=${offset}&total=${total}`;
    return resumableUpload(await apiClient.mutate<unknown>(query, {
      method: "PATCH",
      headers: { "Content-Type": "application/octet-stream" },
      body: chunk,
      ...(signal === undefined ? {} : { signal }),
    }, { locallyHandledCodes: transferProblems }));
  },
  async appendUploadRange(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload> {
    const query = `/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}?path=${encodeURIComponent(remotePath)}&offset=${offset}&total=${total}&range=true&length=${chunk.size}`;
    return resumableUpload(await apiClient.mutate<unknown>(query, {
      method: "PATCH",
      headers: { "Content-Type": "application/octet-stream" },
      body: chunk,
      ...(signal === undefined ? {} : { signal }),
    }, { locallyHandledCodes: transferProblems }));
  },
  async completeUpload(alias: string, id: string, remotePath: string, size: number, expectedRevision: string, sourceFingerprint: string): Promise<void> {
    await postJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}/complete`, { path: remotePath, size, expectedRevision, sourceFingerprint }, undefined, transferProblems);
  },
  async cancelUpload(alias: string, id: string, remotePath: string): Promise<void> {
    await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}?path=${encodeURIComponent(remotePath)}`, {
      method: "DELETE",
    }, { locallyHandledCodes: transferProblems });
  },
  async streamDownload(alias: string, jobId: string, remotePath: string, directory: boolean, offset: number, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }> {
    const headers = !directory && offset > 0
      ? { Range: `bytes=${offset}-`, ...(options.revision === undefined ? {} : { "If-Range": options.revision }) }
      : undefined;
    const endpoint = `${pathFor(alias, directory ? "archive" : "download", remotePath)}&jobId=${encodeURIComponent(jobId)}`;
    const response = await apiClient.send(endpoint, {
      method: "GET", ...(headers === undefined ? {} : { headers }), ...(options.signal === undefined ? {} : { signal: options.signal }),
    }, { locallyHandledCodes: transferProblems });
    if (!response.ok || (!directory && offset > 0 && response.status !== 200 && response.status !== 206)) throw new Error("download_failed");
    const length = Number(response.headers.get("Content-Length"));
    const reset = !directory && offset > 0 && response.status === 200;
    const responseOffset = reset ? 0 : offset;
    const total = Number.isFinite(length) && length >= 0 ? responseOffset + length : null;
    const revision = response.headers.get("ETag");
    if (!directory && revision === null) throw new Error("download_revision_missing");
    if (revision !== null) options.onRevision?.(revision);
    if (reset) await options.onReset?.(total);
    let bytes = responseOffset;
    if (response.body === null) {
      const chunk = new Uint8Array(await (await response.blob()).arrayBuffer());
      bytes += chunk.byteLength;
      await options.onChunk(chunk, total);
      return { bytes, total };
    }
    const reader = response.body.getReader();
    while (true) {
      const next = await reader.read();
      if (next.done) break;
      bytes += next.value.byteLength;
      await options.onChunk(next.value, total);
    }
    return { bytes, total };
  },
  async saveDownload(remotePath: string, directory: boolean, chunks: BlobPart[]): Promise<void> {
    const blob = new Blob(chunks, { type: directory ? "application/zip" : "application/octet-stream" });
    const components = remotePath.split("/").filter(Boolean);
    const name = `${components[components.length - 1] ?? "download"}${directory ? ".zip" : ""}`;
    if (await saveWithAndroid(blob, name)) return;
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = name;
    anchor.click();
    // WebView/Safari may consume the object URL after click() returns.
    globalThis.setTimeout(() => URL.revokeObjectURL(url), 30_000);
  },
  async chmod(alias: string, remotePath: string, mode: string, expectedRevision: string, recursive = false): Promise<RemoteEntry> {
    const target = `${alias}:${remotePath}:${mode}${recursive ? ":recursive" : ""}`;
    const token = await issueAction("sftp.chmod", target);
    return entry(await patchJSON<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/mode`, { path: remotePath, mode, expectedRevision, recursive }, token));
  },
};

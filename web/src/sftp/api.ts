import { apiClient } from "../api/client";
import { asArray, asNumber, asRecord, asString, issueAction, jsonHeaders } from "../api/guards";

export type RemoteEntry = {
  name: string;
  path: string;
  type: "file" | "directory" | "symlink" | "other";
  size: number;
  mode: string;
  modifiedAt: string;
  revision: string;
};

export type RemoteTextFile = {
  entry: RemoteEntry;
  contents: string;
  revision: string;
};

export type TransferOptions = {
  signal?: AbortSignal;
  onProgress?: (bytes: number, total: number | null) => void;
};

export type ResumableUpload = {
  id: string;
  path: string;
  offset: number;
  size: number;
  expectedRevision: string;
};

export type TransferDirection = "upload" | "download";
export type TransferKind = "file" | "folder";
export type TransferJobStatus = "queued" | "running" | "paused" | "reattach" | "needs_overwrite" | "completed" | "failed" | "cancelled";
export type TransferJobAction = "start" | "pause" | "resume" | "retry" | "cancel" | "progress" | "complete" | "fail" | "needs_overwrite";

export type TransferJob = {
  id: string;
  batchId: string;
  alias: string;
  direction: TransferDirection;
  kind: TransferKind;
  name: string;
  remotePath: string;
  totalBytes: number;
  transferredBytes: number;
  bytesPerSecond: number;
  remainingSeconds: number;
  status: TransferJobStatus;
  attempt: number;
  problem: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateTransferJob = Pick<TransferJob, "id" | "batchId" | "alias" | "direction" | "kind" | "name" | "remotePath" | "totalBytes">;

export type StreamDownloadOptions = {
  signal?: AbortSignal;
  onChunk: (chunk: Uint8Array, total: number | null) => void;
};

function pathFor(alias: string, suffix: string, remotePath: string): string {
  return `/api/v1/sftp/${encodeURIComponent(alias)}/${suffix}?path=${encodeURIComponent(remotePath)}`;
}

function entry(value: unknown): RemoteEntry {
  const item = asRecord(value);
  const type = asString(item.type);
  if (type !== "file" && type !== "directory" && type !== "symlink" && type !== "other") {
    throw new Error("invalid_response");
  }
  return {
    name: asString(item.name),
    path: asString(item.path),
    type,
    size: asNumber(item.size),
    mode: asString(item.mode),
    modifiedAt: asString(item.modifiedAt),
    revision: asString(item.revision),
  };
}

function resumableUpload(value: unknown): ResumableUpload {
  const upload = asRecord(value);
  return {
    id: asString(upload.id),
    path: asString(upload.path),
    offset: asNumber(upload.offset),
    size: asNumber(upload.size),
    expectedRevision: asString(upload.expectedRevision),
  };
}

function transferJob(value: unknown): TransferJob {
  const job = asRecord(value);
  const direction = asString(job.direction);
  const kind = asString(job.kind);
  const status = asString(job.status);
  if ((direction !== "upload" && direction !== "download") || (kind !== "file" && kind !== "folder") ||
      !["queued", "running", "paused", "reattach", "needs_overwrite", "completed", "failed", "cancelled"].includes(status)) {
    throw new Error("invalid_response");
  }
  return {
    id: asString(job.id), batchId: asString(job.batchId), alias: asString(job.alias), direction, kind,
    name: asString(job.name), remotePath: asString(job.remotePath), totalBytes: asNumber(job.totalBytes),
    transferredBytes: asNumber(job.transferredBytes), bytesPerSecond: asNumber(job.bytesPerSecond),
    remainingSeconds: asNumber(job.remainingSeconds), status: status as TransferJobStatus,
    attempt: asNumber(job.attempt), problem: asString(job.problem), createdAt: asString(job.createdAt), updatedAt: asString(job.updatedAt),
  };
}

export const sftpApi = {
  async listTransfers(): Promise<{ maxConcurrent: number; jobs: TransferJob[] }> {
    const value = asRecord(await apiClient.read("/api/v1/sftp/transfers"));
    return { maxConcurrent: asNumber(value.maxConcurrent), jobs: asArray(value.jobs).map(transferJob) };
  },
  async createTransfer(input: CreateTransferJob): Promise<TransferJob> {
    return transferJob(await apiClient.mutate<unknown>("/api/v1/sftp/transfers", {
      method: "POST", headers: jsonHeaders, body: JSON.stringify(input),
    }));
  },
  async updateTransfer(id: string, action: TransferJobAction, options: { transferredBytes?: number; problem?: string; resetProgress?: boolean } = {}): Promise<TransferJob> {
    return transferJob(await apiClient.mutate<unknown>(`/api/v1/sftp/transfers/${encodeURIComponent(id)}/actions`, {
      method: "POST", headers: jsonHeaders, body: JSON.stringify({ action, ...options }),
    }));
  },
  async list(alias: string, remotePath: string): Promise<{ path: string; entries: RemoteEntry[] }> {
    const value = asRecord(await apiClient.read(pathFor(alias, "entries", remotePath)));
    return { path: asString(value.path), entries: asArray(value.entries).map(entry) };
  },
  async readText(alias: string, remotePath: string): Promise<RemoteTextFile> {
    const value = asRecord(await apiClient.read(pathFor(alias, "text", remotePath)));
    return { entry: entry(value.entry), contents: asString(value.contents), revision: asString(value.revision) };
  },
  async saveText(alias: string, remotePath: string, contents: string, expectedRevision: string): Promise<RemoteTextFile> {
    const value = asRecord(await apiClient.mutate<unknown>(pathFor(alias, "text", remotePath), {
      method: "PUT",
      headers: jsonHeaders,
      body: JSON.stringify({ contents, expectedRevision }),
    }));
    return { entry: entry(value.entry), contents: asString(value.contents), revision: asString(value.revision) };
  },
  async mkdir(alias: string, remotePath: string): Promise<RemoteEntry> {
    return entry(await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/entries`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ path: remotePath, type: "directory" }),
    }));
  },
  async rename(alias: string, from: string, to: string): Promise<RemoteEntry> {
    return entry(await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/entry`, {
      method: "PATCH",
      headers: jsonHeaders,
      body: JSON.stringify({ from, to }),
    }));
  },
  async remove(alias: string, remotePath: string): Promise<void> {
    const target = `${alias}:${remotePath}`;
    const token = await issueAction("sftp.delete", target);
    const response = await apiClient.send(pathFor(alias, "entry", remotePath), {
      method: "DELETE",
      headers: { "X-SSHC-Action": token },
    });
    if (!response.ok) throw new Error("delete_failed");
  },
  async upload(alias: string, remotePath: string, file: File, overwrite: boolean, signal?: AbortSignal): Promise<void> {
    const query = `${pathFor(alias, "upload", remotePath)}&overwrite=${overwrite ? "true" : "false"}`;
    await apiClient.mutate<unknown>(query, {
      method: "POST",
      headers: { "Content-Type": "application/octet-stream", "X-SSHC-Filename": encodeURIComponent(file.name) },
      body: file,
      ...(signal === undefined ? {} : { signal }),
    });
  },
  async startUpload(alias: string, id: string, remotePath: string, size: number, overwrite: boolean, expectedRevision = ""): Promise<ResumableUpload> {
    return resumableUpload(await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ path: remotePath, size, overwrite, ...(expectedRevision === "" ? {} : { expectedRevision }) }),
    }));
  },
  async appendUpload(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload> {
    const query = `/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}?path=${encodeURIComponent(remotePath)}&offset=${offset}&total=${total}`;
    return resumableUpload(await apiClient.mutate<unknown>(query, {
      method: "PATCH",
      headers: { "Content-Type": "application/octet-stream" },
      body: chunk,
      ...(signal === undefined ? {} : { signal }),
    }));
  },
  async completeUpload(alias: string, id: string, remotePath: string, size: number, expectedRevision: string): Promise<void> {
    await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}/complete`, {
      method: "POST",
      headers: jsonHeaders,
      body: JSON.stringify({ path: remotePath, size, expectedRevision }),
    });
  },
  async cancelUpload(alias: string, id: string, remotePath: string): Promise<void> {
    await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/uploads/${encodeURIComponent(id)}?path=${encodeURIComponent(remotePath)}`, {
      method: "DELETE",
    });
  },
  async streamDownload(alias: string, remotePath: string, directory: boolean, offset: number, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }> {
    const headers = !directory && offset > 0 ? { Range: `bytes=${offset}-` } : undefined;
    const response = await apiClient.send(pathFor(alias, directory ? "archive" : "download", remotePath), {
      method: "GET", ...(headers === undefined ? {} : { headers }), ...(options.signal === undefined ? {} : { signal: options.signal }),
    });
    if (!response.ok || (!directory && offset > 0 && response.status !== 206)) throw new Error("download_failed");
    const length = Number(response.headers.get("Content-Length"));
    const total = Number.isFinite(length) && length >= 0 ? offset + length : null;
    let bytes = offset;
    if (response.body === null) {
      const chunk = new Uint8Array(await (await response.blob()).arrayBuffer());
      bytes += chunk.byteLength;
      options.onChunk(chunk, total);
      return { bytes, total };
    }
    const reader = response.body.getReader();
    while (true) {
      const next = await reader.read();
      if (next.done) break;
      bytes += next.value.byteLength;
      options.onChunk(next.value, total);
    }
    return { bytes, total };
  },
  saveDownload(remotePath: string, directory: boolean, chunks: Uint8Array[]): void {
    const blob = new Blob(chunks as BlobPart[], { type: directory ? "application/zip" : "application/octet-stream" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${remotePath.split("/").filter(Boolean).at(-1) ?? "download"}${directory ? ".zip" : ""}`;
    anchor.click();
    URL.revokeObjectURL(url);
  },
  async download(alias: string, remotePath: string, directory = false, options: TransferOptions = {}): Promise<number> {
    const chunks: Uint8Array[] = [];
    let bytes = 0;
    let total: number | null = null;
    let failures = 0;
    while (true) {
      try {
        const streamed = await this.streamDownload(alias, remotePath, directory, bytes, {
          ...(options.signal === undefined ? {} : { signal: options.signal }),
          onChunk: (chunk, reportedTotal) => {
            chunks.push(chunk);
            bytes += chunk.byteLength;
            total = reportedTotal ?? total;
            options.onProgress?.(bytes, total);
          },
        });
        bytes = streamed.bytes;
        total = streamed.total ?? total;
        break;
      } catch (error) {
        if (directory || options.signal?.aborted || failures >= 2) throw error;
        failures += 1;
      }
    }
    this.saveDownload(remotePath, directory, chunks);
    return bytes;
  },
  async chmod(alias: string, remotePath: string, mode: string, expectedRevision: string): Promise<RemoteEntry> {
    const target = `${alias}:${remotePath}:${mode}`;
    const token = await issueAction("sftp.chmod", target);
    return entry(await apiClient.mutate<unknown>(`/api/v1/sftp/${encodeURIComponent(alias)}/mode`, {
      method: "PATCH",
      headers: { ...jsonHeaders, "X-SSHC-Action": token },
      body: JSON.stringify({ path: remotePath, mode, expectedRevision }),
    }));
  },
};

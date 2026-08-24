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

export const sftpApi = {
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
  async download(alias: string, remotePath: string, directory = false, options: TransferOptions = {}): Promise<number> {
    const response = await apiClient.send(pathFor(alias, directory ? "archive" : "download", remotePath), { method: "GET", ...(options.signal === undefined ? {} : { signal: options.signal }) });
    if (!response.ok) throw new Error("download_failed");
    const totalHeader = Number(response.headers.get("Content-Length"));
    const total = Number.isFinite(totalHeader) && totalHeader > 0 ? totalHeader : null;
    let blob: Blob;
    let bytes = 0;
    if (response.body === null) {
      blob = await response.blob();
      bytes = blob.size;
      options.onProgress?.(bytes, total);
    } else {
      const reader = response.body.getReader();
      const chunks: Uint8Array[] = [];
      while (true) {
        const next = await reader.read();
        if (next.done) break;
        chunks.push(next.value);
        bytes += next.value.byteLength;
        options.onProgress?.(bytes, total);
      }
      blob = new Blob(chunks as BlobPart[], { type: directory ? "application/zip" : "application/octet-stream" });
    }
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${remotePath.split("/").filter(Boolean).at(-1) ?? "download"}${directory ? ".zip" : ""}`;
    anchor.click();
    URL.revokeObjectURL(url);
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

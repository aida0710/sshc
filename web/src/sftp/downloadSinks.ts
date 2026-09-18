import type { ManagedTransferJob } from "./transferLedger";

export type DownloadSink = {
  root: FileSystemDirectoryHandle;
  name: string;
  handle: FileSystemFileHandle;
  writer: FileSystemWritableFileStream;
  position: number;
  // The stored prefix could not be trusted and the download restarts from 0.
  reset: boolean;
};

function partName(id: string): string {
  return `sshc-sftp-${id}.part`;
}

function opfsRoot(): Promise<FileSystemDirectoryHandle> | null {
  if (typeof globalThis.navigator?.storage?.getDirectory !== "function") return null;
  return globalThis.navigator.storage.getDirectory();
}

// Partial downloads land in the origin's private file system so a reload
// can pick up where it left off. Without that storage open() yields null
// and the caller buffers in memory instead.
export class DownloadSinkStore {
  private readonly sinks = new Map<string, DownloadSink>();

  get(id: string): DownloadSink | undefined {
    return this.sinks.get(id);
  }

  async open(job: ManagedTransferJob): Promise<DownloadSink | null> {
    const existing = this.sinks.get(job.id);
    if (existing !== undefined) return existing;
    try {
      const root = await opfsRoot();
      if (root === null) return null;
      const name = partName(job.id);
      const handle = await root.getFileHandle(name, { create: true });
      const writer = await handle.createWritable({ keepExistingData: true });
      const storedSize = (await handle.getFile()).size;
      const invalidStoredSize = storedSize < 0 || (job.totalBytes >= 0 && storedSize > job.totalBytes);
      const reset = invalidStoredSize || (storedSize !== job.transferredBytes && job.downloadRevision === "");
      if (reset) await writer.truncate(0);
      const position = reset ? 0 : storedSize;
      await writer.seek(position);
      const sink = { root, name, handle, writer, reset, position };
      this.sinks.set(job.id, sink);
      return sink;
    } catch {
      return null;
    }
  }

  // Flags a sink so that the next run throws its prefix away.
  markReset(id: string): void {
    const sink = this.sinks.get(id);
    if (sink !== undefined) sink.reset = true;
  }

  // Stops tracking a sink whose file the caller has finished with.
  release(id: string): void {
    this.sinks.delete(id);
  }

  // Closes and deletes the part file, whether or not this store opened it.
  async discard(id: string): Promise<void> {
    const sink = this.sinks.get(id);
    this.sinks.delete(id);
    if (sink !== undefined) {
      await sink.writer.close().catch(() => undefined);
      await sink.root.removeEntry(sink.name).catch(() => undefined);
      return;
    }
    try {
      const root = await opfsRoot();
      await root?.removeEntry(partName(id)).catch(() => undefined);
    } catch { /* OPFS is optional. */ }
  }

  // Deletes part files whose job the engine no longer lists as unfinished.
  async removeOrphans(active: Set<string>): Promise<void> {
    try {
      const root = await opfsRoot();
      if (root === null) return;
      const entries = (root as unknown as { values(): AsyncIterable<FileSystemHandle> }).values();
      for await (const entry of entries) {
        const match = /^sshc-sftp-(.+)\.part$/.exec(entry.name);
        const id = match?.[1];
        if (id !== undefined && !active.has(id)) await root.removeEntry(entry.name).catch(() => undefined);
      }
    } catch { /* OPFS is optional. */ }
  }

  // Makes the bytes written so far durable and confirms the file agrees
  // with the position before the engine is told about it.
  async checkpoint(sink: DownloadSink): Promise<void> {
    await sink.writer.close();
    const committed = await sink.handle.getFile();
    if (committed.size !== sink.position) throw new Error("sftp_download_checkpoint_failed");
    sink.writer = await sink.handle.createWritable({ keepExistingData: true });
    await sink.writer.seek(sink.position);
  }
}

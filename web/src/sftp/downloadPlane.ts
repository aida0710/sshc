import { DownloadSinkStore, type DownloadSink } from "./downloadSinks";
import type { TransferManagerAPI, TransferPlaneContext } from "./transferPlane";

type DownloadAPI = Pick<TransferManagerAPI, "updateTransfer" | "checkpointDownload" | "verifyDownload" | "streamDownload" | "saveDownload">;

// Downloads buffered in memory stop here; beyond that only a sink can hold them.
const fallbackDownloadLimit = 64 << 20;

// Streams remote files into the browser and hands them to the save dialog.
// Bytes go to a durable sink when the browser offers one, otherwise to an
// in-memory buffer; either way the engine is told the checkpoint reached.
export class DownloadPlane {
  private readonly chunks = new Map<string, Uint8Array[]>();
  private readonly sinks = new DownloadSinkStore();

  constructor(private readonly api: DownloadAPI, private readonly context: TransferPlaneContext) {}

  // A new job starts with an empty buffer.
  prepare(id: string): void {
    this.chunks.set(id, []);
  }

  // A folder cannot resume mid-archive: the next run starts from zero.
  restart(id: string): void {
    this.chunks.set(id, []);
    this.sinks.markReset(id);
  }

  // Drops the buffer and the part file of a job that is gone or cancelled.
  async discard(id: string): Promise<void> {
    this.chunks.delete(id);
    await this.sinks.discard(id);
  }

  removeOrphans(active: Set<string>): Promise<void> {
    return this.sinks.removeOrphans(active);
  }

  async run(id: string): Promise<void> {
    const { ledger } = this.context;
    let job = ledger.find(id);
    if (job === undefined) return;
    const sink = await this.sinks.open(job);
    const chunks = this.chunks.get(id) ?? [];
    this.chunks.set(id, chunks);
    let bufferedBytes = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0);

    if (sink !== null && sink.position !== job.transferredBytes && job.downloadRevision !== "") {
      try {
        await this.api.checkpointDownload(id, sink.position, job.downloadRevision);
        ledger.replace(id, { transferredBytes: sink.position });
        job = ledger.find(id)!;
      } catch {
        await sink.writer.truncate(0);
        await sink.writer.seek(0);
        sink.position = 0;
        sink.reset = true;
      }
    }
    const lostFallbackPrefix = sink === null && job.transferredBytes > 0 &&
      (bufferedBytes !== job.transferredBytes || job.downloadRevision === "");
    if (lostFallbackPrefix || sink?.reset === true) {
      chunks.length = 0;
      bufferedBytes = 0;
      if (sink !== null) sink.reset = false;
      ledger.replace(id, { transferredBytes: 0, downloadRevision: "", bytesPerSecond: 0, remainingSeconds: -1 });
      await this.api.updateTransfer(id, "progress", { transferredBytes: 0, resetProgress: true });
      job = ledger.find(id);
      if (job === undefined) return;
    }

    const alreadyComplete = sink !== null && job.kind === "file" && job.downloadRevision !== "" &&
      job.totalBytes >= 0 && sink.position === job.totalBytes;
    let shouldStream = !alreadyComplete;
    if (alreadyComplete) {
      try {
        await this.api.verifyDownload(job.alias, id, job.remotePath, job.downloadRevision);
        await this.api.checkpointDownload(id, sink.position, job.downloadRevision);
        ledger.replace(id, { transferredBytes: sink.position });
        job = ledger.find(id)!;
      } catch {
        await sink.writer.truncate(0);
        await sink.writer.seek(0);
        sink.position = 0;
        await this.sinks.checkpoint(sink);
        ledger.replace(id, { transferredBytes: 0, downloadRevision: "", bytesPerSecond: 0, remainingSeconds: -1 });
        await this.api.updateTransfer(id, "progress", { transferredBytes: 0, resetProgress: true });
        job = ledger.find(id)!;
        shouldStream = true;
      }
    }
    if (shouldStream) bufferedBytes = await this.stream(id, sink, chunks, bufferedBytes);
    job = ledger.find(id);
    if (job === undefined || job.status !== "running") return;
    if (sink !== null) {
      await sink.writer.close();
      this.sinks.release(id);
      const file = await sink.handle.getFile();
      await this.api.saveDownload(job.remotePath, job.kind === "folder", [file]);
      globalThis.setTimeout(() => { void sink.root.removeEntry(sink.name).catch(() => undefined); }, 30_000);
    } else {
      await this.api.saveDownload(job.remotePath, job.kind === "folder", chunks.map((chunk) => new Uint8Array(chunk)));
      if (job.downloadRevision === "") throw new Error("download_revision_missing");
      const acknowledged = await this.api.checkpointDownload(id, bufferedBytes, job.downloadRevision);
      this.context.progress(id, bufferedBytes, acknowledged.totalBytes >= 0 ? acknowledged.totalBytes : bufferedBytes);
    }
    const completed = await this.api.updateTransfer(id, "complete");
    this.chunks.delete(id);
    ledger.replaceServer(completed);
    ledger.notify(completed);
  }

  // Pulls the rest of the file from the engine's current offset and returns
  // how much the in-memory buffer holds afterwards. A file download gets two
  // more tries after a dropped connection; a folder archive cannot be
  // resumed and fails at once.
  private async stream(id: string, sink: DownloadSink | null, chunks: Uint8Array[], buffered: number): Promise<number> {
    const { ledger } = this.context;
    let bufferedBytes = buffered;
    let failures = 0;
    let responseRevision = ledger.find(id)?.downloadRevision ?? "";
    while (true) {
      const job = ledger.find(id);
      if (job === undefined || job.status !== "running") return bufferedBytes;
      const controller = this.context.arm(id);
      try {
        await this.api.streamDownload(job.alias, id, job.remotePath, job.kind === "folder", job.transferredBytes, {
          ...(job.downloadRevision === "" ? {} : { revision: job.downloadRevision }),
          signal: controller.signal,
          onRevision: (revision) => {
            responseRevision = revision;
            ledger.replace(id, { downloadRevision: revision });
          },
          onReset: async (total) => {
            chunks.length = 0;
            bufferedBytes = 0;
            if (sink !== null) {
              await sink.writer.truncate(0);
              await sink.writer.seek(0);
              sink.position = 0;
              await this.sinks.checkpoint(sink);
            }
            const totalBytes = total ?? ledger.find(id)?.totalBytes ?? -1;
            ledger.replace(id, { transferredBytes: 0, totalBytes, bytesPerSecond: 0, remainingSeconds: -1 });
          },
          onChunk: async (chunk, total) => {
            if (responseRevision === "") throw new Error("download_revision_missing");
            let position: number;
            if (sink !== null) {
              await sink.writer.write(new Uint8Array(chunk));
              sink.position += chunk.byteLength;
              await this.sinks.checkpoint(sink);
              position = sink.position;
            } else {
              if (bufferedBytes + chunk.byteLength > fallbackDownloadLimit) {
                throw new Error("sftp_download_storage_unsupported");
              }
              chunks.push(chunk);
              bufferedBytes += chunk.byteLength;
              position = bufferedBytes;
            }
            const acknowledged = await this.api.checkpointDownload(id, position, responseRevision);
            if (ledger.find(id) !== undefined) {
              this.context.progress(id, position, total ?? acknowledged.totalBytes);
            }
          },
        });
        return bufferedBytes;
      } catch (error) {
        const current = ledger.find(id);
        if (current === undefined || current.status !== "running" || controller.signal.aborted) throw error;
        if (current.kind === "folder" || failures >= 2) throw error;
        failures += 1;
      }
    }
  }
}

import { DownloadSinkStore, type DownloadSink } from "./downloadSinks";
import type { TransferManagerAPI, TransferPlaneContext } from "./transferPlane";

type DownloadAPI = Pick<TransferManagerAPI, "updateTransfer" | "checkpointDownload" | "verifyDownload" | "streamDownload" | "saveDownload">;

// Downloads buffered in memory stop here; beyond that only a sink can hold them.
const fallbackDownloadLimit = 64 << 20;
// How many received bytes may wait before the part file is committed and the
// engine told about them. Larger batches cost at most this much re-download
// after a crash; smaller ones cost a file copy and a request each.
const downloadCheckpointBytes = 8 << 20;

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
  //
  // Received bytes are checkpointed in batches. A checkpoint commits the part
  // file and tells the engine the offset the browser now holds durably, and
  // committing an OPFS writable copies the whole file so far; doing that for
  // every chunk made a download take time proportional to the square of its
  // size. The final and the pre-retry checkpoint keep the engine's offset in
  // step with the bytes actually kept.
  private async stream(id: string, sink: DownloadSink | null, chunks: Uint8Array[], buffered: number): Promise<number> {
    const { ledger } = this.context;
    let bufferedBytes = buffered;
    let failures = 0;
    let responseRevision = ledger.find(id)?.downloadRevision ?? "";
    let position = sink === null ? bufferedBytes : sink.position;
    let uncheckpointedBytes = 0;
    let knownTotal: number | null = null;

    const totalFor = (total: number | null) => total ?? knownTotal ?? ledger.find(id)?.totalBytes ?? -1;
    const checkpoint = async (total: number | null) => {
      if (responseRevision === "") throw new Error("download_revision_missing");
      if (sink !== null) await this.sinks.checkpoint(sink);
      const acknowledged = await this.api.checkpointDownload(id, position, responseRevision);
      uncheckpointedBytes = 0;
      if (ledger.find(id) !== undefined) {
        this.context.progress(id, position, total ?? (acknowledged.totalBytes >= 0 ? acknowledged.totalBytes : totalFor(null)));
      }
    };

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
            position = 0;
            uncheckpointedBytes = 0;
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
            knownTotal = total;
            if (sink !== null) {
              await sink.writer.write(new Uint8Array(chunk));
              sink.position += chunk.byteLength;
              position = sink.position;
            } else {
              if (bufferedBytes + chunk.byteLength > fallbackDownloadLimit) {
                throw new Error("sftp_download_storage_unsupported");
              }
              chunks.push(chunk);
              bufferedBytes += chunk.byteLength;
              position = bufferedBytes;
            }
            uncheckpointedBytes += chunk.byteLength;
            if (uncheckpointedBytes >= downloadCheckpointBytes) {
              await checkpoint(total);
              return;
            }
            if (ledger.find(id) !== undefined) this.context.progress(id, position, totalFor(total));
          },
        });
        if (uncheckpointedBytes > 0) await checkpoint(knownTotal);
        return bufferedBytes;
      } catch (error) {
        const current = ledger.find(id);
        if (current === undefined || current.status !== "running" || controller.signal.aborted) throw error;
        if (current.kind === "folder" || failures >= 2) throw error;
        // The retry asks the engine to continue from the bytes held here, so
        // the engine must have been told about them first.
        if (uncheckpointedBytes > 0) await checkpoint(knownTotal);
        failures += 1;
      }
    }
  }
}

import { retryableTransferFailure, TransferPublicationUncertain } from "./transferRecovery";
import type { BrowserSave } from "./api";
import { downloadCheckpointInterval } from "./downloadCheckpointInterval";
import { DownloadSinkStore, type DownloadSink } from "./downloadSinks";
import type { ManagedTransferJob } from "./transferLedger";
import type { TransferManagerAPI, TransferPlaneContext } from "./transferPlane";

type DownloadAPI = Pick<TransferManagerAPI, "updateTransfer" | "checkpointDownload" | "verifyDownload" | "streamDownload" | "saveDownload">;

// Downloads buffered in memory stop here; beyond that only a sink can hold them.
const fallbackDownloadLimit = 64 << 20;

// How long a finished download's part file and object URL outlive the moment
// they were handed to the browser's save. The browser reads them right after
// click(), at local disk speed, and never says when it has finished; ten
// minutes covers copying tens of gigabytes. Without this bound they would stay
// as long as the job is listed, which with the auto-clear setting at 0 means
// until the list is cleared by hand.
export const browserSaveRetentionMs = 10 * 60_000;

// A browser's save this page still backs, and when it was handed over.
type HeldBrowserSave = { save: BrowserSave; handedAt: number };

// Streams remote files into the browser and hands them to the save dialog.
// Bytes go to a durable sink when the browser offers one, otherwise to an
// in-memory buffer; either way the engine is told the checkpoint reached.
export class DownloadPlane {
  private readonly chunks = new Map<string, Uint8Array[]>();
  private readonly sinks = new DownloadSinkStore();
  // Finished downloads that this page handed to the browser's save, which may
  // still be reading them. Their part file and object URL stay until the job
  // leaves the list or browserSaveRetentionMs has passed; what a closed page
  // leaves behind, the next page's first reconcile deletes.
  private readonly browserSaves = new Map<string, HeldBrowserSave>();

  constructor(
    private readonly api: DownloadAPI,
    private readonly context: TransferPlaneContext,
    private readonly now: () => number,
  ) {}

  // A new job starts with an empty buffer.
  prepare(id: string): void {
    this.chunks.set(id, []);
  }

  // A folder cannot resume mid-archive: the next run starts from zero.
  restart(id: string): void {
    this.chunks.set(id, []);
    this.sinks.markReset(id);
  }

  // Drops the buffer, the browser's save and the part file of a job that is
  // gone or cancelled.
  async discard(id: string): Promise<void> {
    this.chunks.delete(id);
    this.releaseBrowserSave(id);
    await this.sinks.discard(id);
  }

  // Lets go of what nothing will read again: the browser's saves of jobs the
  // engine no longer lists or that were handed over browserSaveRetentionMs
  // ago, and the part files of those jobs and of finished ones, except a
  // finished one whose save this page still holds.
  removeOrphans(listed: readonly ManagedTransferJob[]): Promise<void> {
    const listedIds = new Set(listed.map((job) => job.id));
    const now = this.now();
    for (const [id, held] of [...this.browserSaves]) {
      if (!listedIds.has(id) || now - held.handedAt >= browserSaveRetentionMs) this.releaseBrowserSave(id);
    }
    return this.sinks.removeOrphans(new Set(listed
      .filter((job) => !["completed", "cancelled"].includes(job.status) || this.browserSaves.has(job.id))
      .map((job) => job.id)));
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
        // The engine refuses an offset beyond what it recorded as sent, which is
        // what the part file holds after the engine restarted mid-download. The
        // engine's own offset for the same revision is still durable, so cutting
        // the part file back to it resumes with a Range request. Only a part file
        // shorter than that offset has to start over.
        const engineOffsetHeld = job.transferredBytes > 0 && sink.position > job.transferredBytes;
        const kept = engineOffsetHeld ? job.transferredBytes : 0;
        await sink.writer.truncate(kept);
        await sink.writer.seek(kept);
        sink.position = kept;
        sink.reset = !engineOffsetHeld;
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
        await this.api.verifyDownload({ alias: job.alias, jobId: id, remotePath: job.remotePath }, job.downloadRevision);
        await this.api.checkpointDownload(id, sink.position, job.downloadRevision);
        ledger.replace(id, { transferredBytes: sink.position });
        job = ledger.find(id)!;
      } catch (error) {
        if ((ledger.find(id)?.reconnectAttempt ?? 0) > 0 || retryableTransferFailure(error)) throw error;
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
    }
    const parts = sink !== null ? [await sink.handle.getFile()] : chunks.map((chunk) => new Uint8Array(chunk));
    let save;
    try { save = await this.api.saveDownload(job.remotePath, job.kind === "folder", parts); }
    catch (error) {
      if (retryableTransferFailure(error)) throw new TransferPublicationUncertain();
      throw error;
    }
    if (save === null) await this.sinks.discard(id);
    else this.browserSaves.set(id, { save, handedAt: this.now() });
    let completed;
    try { completed = await this.api.updateTransfer(id, "complete"); }
    catch { await this.context.reconcile().catch(() => undefined); throw new TransferPublicationUncertain(); }
    this.chunks.delete(id);
    ledger.replaceServer(completed);
    ledger.notify(completed);
  }

  private releaseBrowserSave(id: string): void {
    this.browserSaves.get(id)?.save.release();
    this.browserSaves.delete(id);
  }

  // Pulls the rest of the file from the engine's current offset and returns
  // how much the in-memory buffer holds afterwards. The manager owns the
  // bounded reconnect policy; a folder archive restarts from byte zero.
  //
  // Received bytes are checkpointed in batches that grow with the committed
  // position (downloadCheckpointInterval). A checkpoint commits the part file
  // and tells the engine the offset the browser now holds durably, and
  // committing an OPFS writable copies the whole file so far, so any fixed
  // batch would make the copies grow with the square of the file size. The
  // final and the pre-retry checkpoint keep the engine's offset in step with
  // the bytes actually kept.
  private async stream(id: string, sink: DownloadSink | null, chunks: Uint8Array[], buffered: number): Promise<number> {
    const { ledger } = this.context;
    let bufferedBytes = buffered;
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
        await this.api.streamDownload({
          alias: job.alias, jobId: id, remotePath: job.remotePath, directory: job.kind === "folder", offset: job.transferredBytes,
        }, {
          ...(job.downloadRevision === "" ? {} : { revision: job.downloadRevision }),
          signal: controller.signal,
          onRevision: (revision) => {
            responseRevision = revision;
            ledger.replace(id, { downloadRevision: revision });
          },
          onReset: async (total) => {
            if ((ledger.find(id)?.reconnectAttempt ?? 0) > 0) throw new Error("sftp_conflict");
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
            const committedPosition = position - uncheckpointedBytes;
            if (uncheckpointedBytes >= downloadCheckpointInterval(committedPosition)) {
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
        if (!retryableTransferFailure(error)) throw error;
        // The retry asks the engine to continue from the bytes held here, so
        // the engine must have been told about them first.
        if (uncheckpointedBytes > 0) await checkpoint(knownTotal);
        throw error;
      }
    }
  }
}

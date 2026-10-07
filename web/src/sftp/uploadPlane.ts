import type { ResumableUpload } from "./api";
import { type TransferManagerAPI, type TransferPlaneContext } from "./transferPlane";
import { TransferPublicationUncertain } from "./transferRecovery";
import { uploadChunkBytes } from "./uploadFingerprint";

type UploadAPI = Pick<TransferManagerAPI, "startUpload" | "appendUpload" | "completeUpload">;

// Pushes the browser's File objects to the engine. Files only live in this
// tab, so a job the engine still lists after a reload waits here until the
// user picks the same file again.
export class UploadPlane {
  private readonly files = new Map<string, File>();

  constructor(private readonly api: UploadAPI, private readonly context: TransferPlaneContext) {}

  has(id: string): boolean {
    return this.files.has(id);
  }

  get(id: string): File | undefined {
    return this.files.get(id);
  }

  attach(id: string, file: File): void {
    this.files.set(id, file);
  }

  detach(id: string): void {
    this.files.delete(id);
  }

  async run(id: string, sourceFingerprint: string): Promise<void> {
    const { ledger } = this.context;
    const file = this.files.get(id);
    let job = ledger.find(id);
    if (file === undefined || job === undefined) return;
    const controller = this.context.arm(id);
    const started = await this.api.startUpload({ alias: job.alias, id, remotePath: job.remotePath, size: job.totalBytes, sourceFingerprint, signal: controller.signal });
    ledger.replace(id, { transferredBytes: started.offset, expectedRevision: started.expectedRevision });
    if (started.parallelism > 1) {
      await this.runParallel(id, file, started, sourceFingerprint);
      return;
    }
    let offset = started.offset;
    while (offset < file.size) {
      job = ledger.find(id);
      if (job === undefined || job.status !== "running") return;
      const controller = this.context.arm(id);
      const end = Math.min(offset + uploadChunkBytes, file.size);
      const appended = await this.api.appendUpload({
        alias: job.alias, id, remotePath: job.remotePath, offset, total: file.size,
        chunk: file.slice(offset, end), range: false, signal: controller.signal,
      });
      offset = appended.offset;
      this.context.progress(id, offset, file.size);
    }
    job = ledger.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.complete(id, job.expectedRevision, sourceFingerprint);
  }

  // Large files go up as fixed ranges over several requests at once; ranges
  // the engine already holds from an earlier attempt are skipped.
  private async runParallel(id: string, file: File, started: ResumableUpload, sourceFingerprint: string): Promise<void> {
    const { ledger } = this.context;
    const ranges: Array<{ offset: number; size: number }> = [];
    for (let offset = 0; offset < file.size; offset += started.chunkBytes) {
      const size = Math.min(started.chunkBytes, file.size - offset);
      if (!started.completedRanges.some((range) => offset >= range.offset && offset + size <= range.offset + range.size)) {
        ranges.push({ offset, size });
      }
    }
    const controller = this.context.arm(id);
    let next = 0;
    const worker = async () => {
      while (next < ranges.length) {
        const portion = ranges[next++];
        if (portion === undefined) return;
        const job = ledger.find(id);
        if (job === undefined || job.status !== "running") return;
        const appended = await this.api.appendUpload({
          alias: job.alias, id, remotePath: job.remotePath, offset: portion.offset, total: file.size,
          chunk: file.slice(portion.offset, portion.offset + portion.size), range: true, signal: controller.signal,
        });
        const current = ledger.find(id)?.transferredBytes ?? 0;
        this.context.progress(id, Math.max(current, appended.offset), file.size);
      }
    };
    const workers = await Promise.allSettled(Array.from({ length: Math.min(started.parallelism, ranges.length) }, async () => {
      try { await worker(); } catch (error) { controller.abort(); throw error; }
    }));
    const failure = workers.find((outcome) => outcome.status === "rejected" && !(outcome.reason instanceof DOMException && outcome.reason.name === "AbortError")) ?? workers.find((outcome) => outcome.status === "rejected");
    if (failure?.status === "rejected") throw failure.reason;
    const job = ledger.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.complete(id, started.expectedRevision, sourceFingerprint);
  }

  private async complete(id: string, expectedRevision: string, sourceFingerprint: string): Promise<void> {
    const { ledger } = this.context;
    const job = ledger.find(id)!;
    const controller = this.context.arm(id);
    try {
      await this.api.completeUpload({ alias: job.alias, id, remotePath: job.remotePath, size: job.totalBytes, expectedRevision, sourceFingerprint, signal: controller.signal });
    } catch (error) {
      await this.context.reconcile().catch(() => undefined);
      if (ledger.find(id)?.status !== "completed") {
        if (error instanceof TypeError) throw new TransferPublicationUncertain();
        throw error;
      }
    }
    this.files.delete(id);
    await this.context.reconcile();
    const completed = ledger.find(id);
    if (completed !== undefined) ledger.notify(completed);
  }
}

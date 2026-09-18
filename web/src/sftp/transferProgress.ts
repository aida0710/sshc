import type { ManagedTransferJob } from "./transferLedger";

type Sample = { at: number; bytes: number };

// Smoothed throughput per job, from the bytes acknowledged since the last
// sample. The estimate leans on history so a burst does not swing the ETA.
export class TransferSpeedometer {
  private readonly samples = new Map<string, Sample>();

  constructor(private readonly now: () => number) {}

  start(id: string, bytes: number): void {
    this.samples.set(id, { at: this.now(), bytes });
  }

  // The progress fields to store for a job that has now reached
  // transferredBytes. A negative totalBytes means "unchanged".
  measure(job: ManagedTransferJob, transferredBytes: number, totalBytes: number): Pick<ManagedTransferJob, "transferredBytes" | "totalBytes" | "bytesPerSecond" | "remainingSeconds"> {
    const at = this.now();
    const sample = this.samples.get(job.id) ?? { at, bytes: job.transferredBytes };
    const elapsed = Math.max((at - sample.at) / 1000, 0);
    let bytesPerSecond = job.bytesPerSecond;
    if (elapsed > 0) {
      const instant = Math.max(0, transferredBytes - sample.bytes) / elapsed;
      bytesPerSecond = bytesPerSecond === 0 ? instant : bytesPerSecond * 0.65 + instant * 0.35;
      this.samples.set(job.id, { at, bytes: transferredBytes });
    }
    const resolvedTotal = totalBytes >= 0 ? totalBytes : job.totalBytes;
    const remainingSeconds = resolvedTotal >= 0 && bytesPerSecond > 0
      ? Math.max(0, Math.round((resolvedTotal - transferredBytes) / bytesPerSecond)) : -1;
    return { transferredBytes, totalBytes: resolvedTotal, bytesPerSecond, remainingSeconds };
  }
}

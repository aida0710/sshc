// The smallest batch of received bytes that is committed at once. Below it a
// checkpoint's file copy and request would cost more than re-downloading the
// batch after a crash.
export const minimumDownloadCheckpointBytes = 8 << 20;

// Each checkpoint may wait for this fraction of the bytes already committed.
// Committing an OPFS writable copies the whole file so far, so a fixed batch
// makes the copies add up to the square of the file size; growing the batch
// with the position keeps the checkpoints geometric and the copies at about
// five times the file size. The price is that a crash re-downloads at most a
// fifth of what had been received.
const checkpointGrowthDivisor = 4;

// How many bytes may arrive after `committedBytes` before the next checkpoint.
export function downloadCheckpointInterval(committedBytes: number): number {
  return Math.max(minimumDownloadCheckpointBytes, Math.floor(committedBytes / checkpointGrowthDivisor));
}

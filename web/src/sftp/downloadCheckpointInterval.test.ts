import { describe, expect, it } from "vitest";
import { downloadCheckpointInterval, minimumDownloadCheckpointBytes } from "./downloadCheckpointInterval";

const mebibyte = 1 << 20;
const gibibyte = 1 << 30;

// Walks a download of `size` bytes checkpoint by checkpoint. Each checkpoint
// copies the whole part file so far, so the copied bytes are the sum of the
// checkpoint positions.
function simulateCheckpoints(size: number): { count: number; copiedBytes: number } {
  let committed = 0;
  let count = 0;
  let copiedBytes = 0;
  while (committed < size) {
    committed = Math.min(size, committed + downloadCheckpointInterval(committed));
    count += 1;
    copiedBytes += committed;
  }
  return { count, copiedBytes };
}

describe("downloadCheckpointInterval", () => {
  it("waits for the minimum batch while little has been committed", () => {
    expect(downloadCheckpointInterval(0)).toBe(minimumDownloadCheckpointBytes);
    expect(downloadCheckpointInterval(16 * mebibyte)).toBe(minimumDownloadCheckpointBytes);
  });

  it("grows the batch with the committed position", () => {
    expect(downloadCheckpointInterval(1 * gibibyte)).toBe(256 * mebibyte);
  });

  it.each([1, 4, 16])("copies about five times a %i GiB file instead of its square", (gibibytes) => {
    const size = gibibytes * gibibyte;
    const { count, copiedBytes } = simulateCheckpoints(size);
    expect(copiedBytes).toBeLessThanOrEqual(6 * size);
    expect(count).toBeLessThan(40);
  });

  it("puts at most a fifth of the received bytes at risk once past the minimum batch", () => {
    for (const committed of [32 * mebibyte, 1 * gibibyte, 4 * gibibyte]) {
      const interval = downloadCheckpointInterval(committed);
      expect(interval).toBeLessThanOrEqual((committed + interval) / 5);
    }
  });
});

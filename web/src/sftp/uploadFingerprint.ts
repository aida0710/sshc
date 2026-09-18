export const uploadChunkBytes = 1 << 20;

// A content hash the engine compares on resume, so a file edited between
// attempts is not stitched together from two versions. Hashing per chunk
// keeps memory flat for large files.
export async function fingerprintFile(file: File, signal?: AbortSignal, running: () => boolean = () => true): Promise<string> {
  const chunkHashes: Uint8Array[] = [];
  for (let offset = 0; offset < file.size; offset += uploadChunkBytes) {
    if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
    chunkHashes.push(new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", await file.slice(offset, offset + uploadChunkBytes).arrayBuffer())));
    if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
  }
  const summary = new Uint8Array(8 + chunkHashes.length * 32);
  const sizeView = new DataView(summary.buffer);
  sizeView.setUint32(0, Math.floor(file.size / 0x1_0000_0000), false);
  sizeView.setUint32(4, file.size % 0x1_0000_0000, false);
  chunkHashes.forEach((hash, index) => summary.set(hash, 8 + index * 32));
  const digest = new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", summary));
  if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
  return `tree-sha256:${[...digest].map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

import { deflateSync } from "node:zlib";

function crc32(bytes: Buffer): number {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit += 1) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1));
  }
  return (crc ^ 0xffffffff) >>> 0;
}

// pngOf encodes an RGB PNG whose rows are all alike, each pixel's colour
// given by its column.
function pngOf(width: number, height: number, colour: (x: number) => readonly [number, number, number]): Buffer {
  const row = Buffer.concat([Buffer.from([0]), ...Array.from({ length: width }, (_, x) => Buffer.from(colour(x)))]);
  const rows = Buffer.concat(Array.from({ length: height }, () => row));
  const chunk = (type: string, data: Buffer) => {
    const length = Buffer.alloc(4);
    length.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
    const checksum = Buffer.alloc(4);
    checksum.writeUInt32BE(crc32(body));
    return Buffer.concat([length, body, checksum]);
  };
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header.set([8, 2, 0, 0, 0], 8);
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header), chunk("IDAT", deflateSync(rows)), chunk("IEND", Buffer.alloc(0)),
  ]);
}

// A 48×24 PNG whose left half is red and right half is blue.
export function samplePNG(): Buffer {
  return pngOf(48, 24, (x) => (x < 24 ? [0xd0, 0x30, 0x30] : [0x30, 0x60, 0xd0]));
}

// The largest image the terminal shows, 4096 × 4096 pixels, in about 350 KB:
// a gradient that compresses well but costs a full decode.
export function largestPNG(): Buffer {
  return pngOf(4096, 4096, (x) => [x & 0xff, (x >> 4) & 0xff, 0x80]);
}

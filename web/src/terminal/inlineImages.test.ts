import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attachInlineImages } from "./inlineImages";
import { createTerminalOutput } from "./output";

// A one-pixel PNG keeps the real addon's base64 decoder and image-type checks in use.
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=";
const pngBytes = Uint8Array.from(atob(pngBase64), (character) => character.charCodeAt(0));
const encoder = new TextEncoder();

function imageHarness() {
  // jsdom cannot decode a bitmap. Inspect the bytes passed to that browser
  // boundary; Playwright covers the resulting pixels on screen.
  const decodeBitmap = vi.fn(async (_source: Blob) => {
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    return canvas;
  });
  vi.stubGlobal("createImageBitmap", decodeBitmap);
  const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
  const images = attachInlineImages(terminal);
  const output = createTerminalOutput(terminal);
  return {
    decodeBitmap,
    output,
    flush: () => new Promise<void>((resolve) => terminal.write("", resolve)),
    dispose() {
      output.dispose();
      images.dispose();
      terminal.dispose();
    },
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("iTerm2 inline images", () => {
  it("drops an image above the addon limit without size and decodes the next image", async () => {
    const subject = imageHarness();
    try {
      // The installed addon caps IIP input at 32 MiB, encoded as base64.
      const maximumImageBytes = 32 * 1024 * 1024;
      const maximumEncodedLength = Math.ceil(maximumImageBytes * 4 / 3);
      // Keep valid base64 and a PNG header so only the input limit rejects it.
      const imagePrefix = pngBase64.slice(0, -4);
      subject.output.write(encoder.encode(`\u001b]1337;File=inline=1:${imagePrefix}`));
      // Stream complete groups instead of allocating another large input string.
      const payloadChunk = encoder.encode("AAAA".repeat(1024));
      for (let sent = imagePrefix.length; sent <= maximumEncodedLength; sent += payloadChunk.length) {
        subject.output.write(payloadChunk);
      }
      subject.output.write(encoder.encode("\u0007"));
      await subject.flush();
      expect(subject.decodeBitmap).not.toHaveBeenCalled();

      subject.output.write(encoder.encode(`\u001b]1337;File=inline=1:${pngBase64}\u0007`));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
    } finally {
      subject.dispose();
    }
  });

  for (const [name, terminator] of [["BEL", "\u0007"], ["ST", "\u001b\\"]] as const) {
    for (const replay of [false, true]) {
      it(`decodes an image without size ending in ${name} from byte-split ${replay ? "replay" : "live output"}`, async () => {
        const subject = imageHarness();
        try {
          const bytes = encoder.encode(`\u001b]1337;File=inline=1:${pngBase64}${terminator}`);
          if (replay) subject.output.beginReplay({ start: 0, next: bytes.length, end: bytes.length, truncated: false });
          for (const byte of bytes) subject.output.write(new Uint8Array([byte]));
          await subject.flush();

          expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
          const blob = subject.decodeBitmap.mock.calls[0]![0];
          expect(blob.type).toBe("image/png");
          expect(new Uint8Array(await blob.arrayBuffer())).toEqual(pngBytes);
        } finally {
          subject.dispose();
        }
      });
    }
  }
});

function toBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

async function zlibCompress(bytes: Uint8Array<ArrayBuffer>): Promise<Uint8Array> {
  const stream = new CompressionStream("deflate");
  const writer = stream.writable.getWriter();
  void writer.write(bytes);
  void writer.close();
  const reader = stream.readable.getReader();
  const parts: Uint8Array[] = [];
  for (let read = await reader.read(); !read.done; read = await reader.read()) parts.push(read.value);
  const compressed = new Uint8Array(parts.reduce((total, part) => total + part.byteLength, 0));
  let offset = 0;
  for (const part of parts) {
    compressed.set(part, offset);
    offset += part.byteLength;
  }
  return compressed;
}

function kittyCommand(control: string, payload: string): Uint8Array {
  return encoder.encode(`\u001b_G${control};${payload}\u001b\\`);
}

// Counts every zlib stream anyone opens, so a test can tell whether the
// image addon started inflating a payload.
function countInflations() {
  const counter = { opened: 0 };
  const Original = DecompressionStream;
  vi.stubGlobal("DecompressionStream", class extends Original {
    constructor(format: CompressionFormat) {
      super(format);
      counter.opened += 1;
    }
  });
  return counter;
}

describe("kitty graphics", () => {
  // jsdom has no ImageData. The addon wraps raw pixels in one before decoding.
  const stubImageData = () => vi.stubGlobal("ImageData", class {
    constructor(readonly data: Uint8ClampedArray, readonly width: number, readonly height: number) {}
  });
  // Far more bytes than any image these tests declare, yet about 1 KiB compressed.
  const inflatedBombBytes = 1024 * 1024;

  it("refuses an image that declares more pixels than the limit before inflating it", async () => {
    stubImageData();
    const inflations = countInflations();
    const subject = imageHarness();
    try {
      const zeros = toBase64(await zlibCompress(new Uint8Array(inflatedBombBytes)));
      subject.output.write(kittyCommand("a=T,f=32,s=20000,v=20000,o=z", zeros));
      await subject.flush();

      expect(inflations.opened).toBe(0);
      expect(subject.decodeBitmap).not.toHaveBeenCalled();
    } finally {
      subject.dispose();
    }
  });

  it("refuses zlib data that inflates past the declared pixels and decodes the next image", async () => {
    stubImageData();
    const inflations = countInflations();
    const subject = imageHarness();
    try {
      const zeros = toBase64(await zlibCompress(new Uint8Array(inflatedBombBytes)));
      subject.output.write(kittyCommand("a=T,f=32,s=2,v=2,o=z", zeros));
      await subject.flush();
      // Only the guard's own bounded stream was opened; the addon never inflated it.
      expect(inflations.opened).toBe(1);
      expect(subject.decodeBitmap).not.toHaveBeenCalled();

      subject.output.write(kittyCommand("a=T,f=100", pngBase64));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
    } finally {
      subject.dispose();
    }
  });

  it("refuses chunked zlib data once it inflates past the limit and decodes the next image", async () => {
    stubImageData();
    const subject = imageHarness();
    try {
      const zeros = toBase64(await zlibCompress(new Uint8Array(inflatedBombBytes)));
      // The first chunk passes the check and reaches the addon before the rest
      // shows how far the data inflates.
      const chunkLength = 8;
      subject.output.write(kittyCommand("a=T,f=32,s=2,v=2,o=z,m=1", zeros.slice(0, chunkLength)));
      for (let offset = chunkLength; offset < zeros.length; offset += chunkLength) {
        const more = offset + chunkLength < zeros.length ? 1 : 0;
        subject.output.write(kittyCommand(`m=${more}`, zeros.slice(offset, offset + chunkLength)));
      }
      await subject.flush();
      expect(subject.decodeBitmap).not.toHaveBeenCalled();

      subject.output.write(kittyCommand("a=T,f=100", pngBase64));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
      const blob = subject.decodeBitmap.mock.calls[0]![0];
      expect(new Uint8Array(await blob.arrayBuffer())).toEqual(pngBytes);
    } finally {
      subject.dispose();
    }
  });

  it("refuses a PNG whose header declares more pixels than the limit and decodes the next image", async () => {
    const subject = imageHarness();
    try {
      const huge = pngBytes.slice();
      const header = new DataView(huge.buffer);
      header.setUint32(16, 30000);
      header.setUint32(20, 30000);
      subject.output.write(kittyCommand("a=T,f=100", toBase64(huge)));
      await subject.flush();
      expect(subject.decodeBitmap).not.toHaveBeenCalled();

      subject.output.write(kittyCommand("a=T,f=100", pngBase64));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
    } finally {
      subject.dispose();
    }
  });

  it("drops an unfinished image when another image starts and decodes the image after", async () => {
    const subject = imageHarness();
    try {
      const compressed = toBase64(await zlibCompress(pngBytes));
      subject.output.write(kittyCommand("a=T,f=100,o=z,m=1", compressed.slice(0, 8)));
      // Not a continuation: the addon would read it as part of the first image.
      subject.output.write(kittyCommand("a=T,f=100", pngBase64));
      await subject.flush();
      expect(subject.decodeBitmap).not.toHaveBeenCalled();

      subject.output.write(kittyCommand("a=T,f=100", pngBase64));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
    } finally {
      subject.dispose();
    }
  });

  it("decodes zlib-compressed pixels that inflate to the declared size", async () => {
    stubImageData();
    const subject = imageHarness();
    try {
      const pixels = new Uint8Array(2 * 2 * 4).fill(0xff);
      subject.output.write(kittyCommand("a=T,f=32,s=2,v=2,o=z", toBase64(await zlibCompress(pixels))));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
    } finally {
      subject.dispose();
    }
  });

  it("decodes a compressed PNG sent in chunks", async () => {
    const subject = imageHarness();
    try {
      const compressed = toBase64(await zlibCompress(pngBytes));
      const middle = Math.floor(compressed.length / 8) * 4;
      subject.output.write(kittyCommand("a=T,f=100,o=z,m=1", compressed.slice(0, middle)));
      subject.output.write(kittyCommand("m=0", compressed.slice(middle)));
      await subject.flush();
      expect(subject.decodeBitmap).toHaveBeenCalledTimes(1);
      const blob = subject.decodeBitmap.mock.calls[0]![0];
      expect(new Uint8Array(await blob.arrayBuffer())).toEqual(pngBytes);
    } finally {
      subject.dispose();
    }
  });
});

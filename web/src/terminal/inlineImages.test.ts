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

import { describe, expect, it } from "vitest";
import { viewportText, type ViewportBuffer } from "./buffer";

function bufferOf(lines: string[], viewportY = 0): ViewportBuffer {
  return {
    viewportY,
    length: lines.length,
    getLine: (index) => {
      const line = lines[index];
      if (line === undefined) return undefined;
      return {
        translateToString: (trimRight?: boolean, start = 0, end = line.length) => {
          const cut = line.slice(start, end);
          return trimRight === true ? cut.trimEnd() : cut;
        },
      };
    },
  };
}

describe("viewportText", () => {
  it("takes the window, not the whole buffer", () => {
    const buffer = bufferOf(["scrolled off", "first", "second", "third"], 1);
    expect(viewportText(buffer, 2, 40)).toBe("first\nsecond");
  });

  it("drops the padding every terminal line carries", () => {
    expect(viewportText(bufferOf(["ls -la      ", "total 0   "]), 2, 40)).toBe("ls -la\ntotal 0");
  });

  it("keeps the blank rows so every line stays on its own line", () => {
    expect(viewportText(bufferOf(["done", "", ""]), 3, 40)).toBe("done\n\n");
  });

  it("clips a line that is still wider than the terminal", () => {
    expect(viewportText(bufferOf(["0123456789"]), 1, 4)).toBe("0123");
  });

  it("answers with a blank line for a row the buffer does not have", () => {
    expect(viewportText(bufferOf(["only"]), 3, 40)).toBe("only\n\n");
  });
});

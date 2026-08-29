import { describe, expect, it } from "vitest";
import { recentBufferText, viewportText, type ViewportBuffer } from "./buffer";

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

describe("recentBufferText", () => {
  it("copies a bounded tail without terminal control characters", () => {
    const buffer = bufferOf(["old", "prompt\u001b[31m", "result\u0007", ""]);
    expect(recentBufferText(buffer, 3)).toBe("prompt[31m\nresult");
  });

  it("joins wrapped physical rows and clips the returned text", () => {
    const lines = ["first", "long ", "line"];
    const buffer: ViewportBuffer = {
      viewportY: 0,
      length: lines.length,
      getLine: (index) => {
        const line = lines[index];
        if (line === undefined) return undefined;
        return {
          isWrapped: index === 2,
          translateToString: () => line,
        };
      },
    };
    expect(recentBufferText(buffer)).toBe("first\nlong line");
    expect(recentBufferText(buffer, 200, 4)).toBe("line");
  });
});

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

  // 端末の行は常に桁数いっぱいの長さを持つ。落とさなければ、選んで貼り付けた
  // ものの行末に空白が何十個も付いてくる。
  it("drops the padding every terminal line carries", () => {
    expect(viewportText(bufferOf(["ls -la      ", "total 0   "]), 2, 40)).toBe("ls -la\ntotal 0");
  });

  // **末尾の空行を落とさない。** 落とすと n 行目が n 行目の上に来なくなる
  // ——重ねる板にとって、それだけが存在理由である。
  it("keeps the blank rows so every line stays on its own line", () => {
    expect(viewportText(bufferOf(["done", "", ""]), 3, 40)).toBe("done\n\n");
  });

  // 行の配列は resize のあと桁数より長いまま残ることがある。渡さないと箱の
  // 外まで字が伸びる。
  it("clips a line that is still wider than the terminal", () => {
    expect(viewportText(bufferOf(["0123456789"]), 1, 4)).toBe("0123");
  });

  it("answers with a blank line for a row the buffer does not have", () => {
    expect(viewportText(bufferOf(["only"]), 3, 40)).toBe("only\n\n");
  });
});

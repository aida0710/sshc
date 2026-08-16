import { describe, expect, it } from "vitest";
import { bufferText, type ReadableBuffer } from "./buffer";

function bufferOf(lines: string[]): ReadableBuffer {
  return {
    length: lines.length,
    getLine: (index) => {
      const line = lines[index];
      if (line === undefined) return undefined;
      return { translateToString: (trimRight?: boolean) => (trimRight ? line.trimEnd() : line) };
    },
  };
}

describe("bufferText", () => {
  it("joins the lines it was given", () => {
    expect(bufferText(bufferOf(["one", "two", "three"]))).toBe("one\ntwo\nthree");
  });

  // 端末の行は常に桁数いっぱいの長さを持つ。落とさなければ、選んで貼り付けた
  // ものの行末に空白が何十個も付いてくる。
  it("drops the padding every terminal line carries", () => {
    expect(bufferText(bufferOf(["ls -la            ", "total 0     "]))).toBe("ls -la\ntotal 0");
  });

  // **まだ何も書かれていない行が数十行あるのが端末の常態である。** そのまま
  // 渡すと、開いた面の大半が空白になる。
  it("stops at the last line that has anything on it", () => {
    expect(bufferText(bufferOf(["done", "", "   ", ""]))).toBe("done");
  });

  it("keeps blank lines that are between two written ones", () => {
    expect(bufferText(bufferOf(["one", "", "two"]))).toBe("one\n\ntwo");
  });

  it("answers with nothing for an empty terminal", () => {
    expect(bufferText(bufferOf([]))).toBe("");
    expect(bufferText(bufferOf(["", ""]))).toBe("");
  });
});

import { describe, expect, it } from "vitest";
import { encodeKey } from "./KeyBar";

describe("encodeKey", () => {
  // 修飾なしの特殊キーは、そのままの制御列である。
  it("sends the control sequences the keys stand for", () => {
    expect(encodeKey("Esc", false, false)).toBe("\x1b");
    expect(encodeKey("Tab", false, false)).toBe("\t");
    expect(encodeKey("↑", false, false)).toBe("\x1b[A");
    expect(encodeKey("↓", false, false)).toBe("\x1b[B");
    expect(encodeKey("→", false, false)).toBe("\x1b[C");
    expect(encodeKey("←", false, false)).toBe("\x1b[D");
  });

  it("passes literal characters through", () => {
    expect(encodeKey("|", false, false)).toBe("|");
    expect(encodeKey("~", false, false)).toBe("~");
  });

  // **Ctrl は sticky である。** 一度押してから次の 1 打鍵に乗る。Ctrl+C が
  // 0x03 にならなければ、走っているものを止められない。
  it("folds ctrl into the control range", () => {
    expect(encodeKey("c", true, false)).toBe("\x03");
    expect(encodeKey("C", true, false)).toBe("\x03");
    expect(encodeKey("d", true, false)).toBe("\x04");
  });

  // Alt は ESC を前置する。端末がそう約束している。
  it("prefixes alt with escape", () => {
    expect(encodeKey("b", false, true)).toBe("\x1bb");
  });

  // 両方立っているときは、Alt の ESC が Ctrl の制御文字の前に来る。
  it("puts the escape before the control character", () => {
    expect(encodeKey("c", true, true)).toBe("\x1b\x03");
  });

  // 特殊キーに Ctrl は乗らない。矢印は矢印のままである。
  it("leaves the special sequences alone when ctrl is held", () => {
    expect(encodeKey("↑", true, false)).toBe("\x1b[A");
  });

  // **押しても何も起きないより、押した文字が出る方がよい。** 触れる画面では、
  // 何も起きないことと修飾が外れていないことが見分けられない。
  it("sends the plain character when ctrl has no meaning for it", () => {
    expect(encodeKey("|", true, false)).toBe("|");
  });
});

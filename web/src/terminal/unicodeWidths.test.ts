import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";

async function written(output: string, current = true) {
  const terminal = new Terminal({ cols: 20, rows: 2, allowProposedApi: true });
  if (current) applyCurrentUnicodeWidths(terminal);
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

async function cellsOf(text: string): Promise<number> {
  const terminal = await written(`${text}|`);
  try {
    return terminal.buffer.active.cursorX - 1;
  } finally { terminal.dispose(); }
}

describe("applyCurrentUnicodeWidths", () => {
  it("絵文字を2マスで数え、後ろの文字がずれない", async () => {
    const terminal = await written("😀X");
    try {
      const line = terminal.buffer.active.getLine(0);
      expect(line?.getCell(0)?.getWidth()).toBe(2);
      expect(line?.getCell(2)?.getChars()).toBe("X");
      expect(terminal.buffer.active.cursorX).toBe(3);
    } finally { terminal.dispose(); }
  });

  it("Unicode 13〜17 で足された絵文字も2マスで数える", async () => {
    // 🥲 (13)、🫠 (14)、🫨 (15)、🫩 (16)、🫪 (17)
    for (const emoji of ["\u{1F972}", "\u{1FAE0}", "\u{1FAE8}", "\u{1FAE9}", "\u{1FAEA}"]) {
      expect(await cellsOf(emoji)).toBe(2);
    }
  });

  it("結合文字と異体字セレクタは前の文字に重ね、マスを増やさない", async () => {
    expect(await cellsOf("é")).toBe(1);
    expect(await cellsOf("が")).toBe(2);
    expect(await cellsOf("❤️")).toBe(1);
    expect(await cellsOf("가")).toBe(2);
  });

  it("全角の文字を2マス、曖昧な幅の文字と制御文字を wcwidth と同じに数える", async () => {
    expect(await cellsOf("日本")).toBe(4);
    expect(await cellsOf("①")).toBe(1);
    expect(await cellsOf("\u{20000}")).toBe(2);
    expect(await cellsOf("a​b")).toBe(2);
  });

  it("xterm.jsの既定（Unicode 6）のままだと絵文字を1マスで数える", async () => {
    const terminal = await written("😀X", false);
    try {
      expect(terminal.buffer.active.getLine(0)?.getCell(1)?.getChars()).toBe("X");
    } finally { terminal.dispose(); }
  });
});

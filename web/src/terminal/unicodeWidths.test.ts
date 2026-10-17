import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { applyUnicode11Widths } from "./unicodeWidths";

async function written(output: string, modern: boolean) {
  const terminal = new Terminal({ cols: 20, rows: 2, allowProposedApi: true });
  if (modern) applyUnicode11Widths(terminal);
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

describe("applyUnicode11Widths", () => {
  it("絵文字を2マスで数え、後ろの文字がずれない", async () => {
    const terminal = await written("😀X", true);
    try {
      const line = terminal.buffer.active.getLine(0);
      expect(line?.getCell(0)?.getWidth()).toBe(2);
      expect(line?.getCell(2)?.getChars()).toBe("X");
      expect(terminal.buffer.active.cursorX).toBe(3);
    } finally { terminal.dispose(); }
  });

  it("xterm.jsの既定（Unicode 6）のままだと絵文字を1マスで数える", async () => {
    const terminal = await written("😀X", false);
    try {
      expect(terminal.buffer.active.getLine(0)?.getCell(1)?.getChars()).toBe("X");
    } finally { terminal.dispose(); }
  });
});

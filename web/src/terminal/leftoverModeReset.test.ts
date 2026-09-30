import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import leftoverModeReset from "../../../internal/terminal/testdata/leftover-mode-reset.json";
import { attachKittyKeyboardProtocol } from "./kittyKeyboard";

// シェルのプロセスが終わるたびにエンジンが出力へ足す列（internal/terminal/leftover_modes.go）を、
// 実際のxterm.jsへ流して確かめる。Goの側はinternal/terminal/leftover_modes_test.goが同じ列を読む。
const reset = leftoverModeReset.sequence;

function writeTo(terminal: Terminal, output: string): Promise<void> {
  return new Promise((resolve) => terminal.write(output, resolve));
}

function cursorOf(terminal: Terminal) {
  return { x: terminal.buffer.active.cursorX, y: terminal.buffer.active.cursorY };
}

describe("the sequence the engine appends when a shell process ends", () => {
  it("stops reporting mouse, focus and paste to the next shell", async () => {
    const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    try {
      await writeTo(terminal, "\u001b[?1003h\u001b[?1006h\u001b[?1004h\u001b[?2004h\u001b[?1h\u001b=\u001b[?2026h");
      expect(terminal.modes.mouseTrackingMode).toBe("any");

      await writeTo(terminal, reset);

      expect(terminal.modes).toMatchObject({
        mouseTrackingMode: "none",
        sendFocusMode: false,
        bracketedPasteMode: false,
        applicationCursorKeysMode: false,
        applicationKeypadMode: false,
        synchronizedOutputMode: false,
      });
    } finally {
      terminal.dispose();
    }
  });

  it("keeps the cursor where it was on the normal screen, even with an older saved cursor", async () => {
    const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    try {
      // ESC 7で左上を保存したあと、シェルが先へ進んだ状態である。
      await writeTo(terminal, "\u001b7line1\r\nline2\r\n$ \u001b[31m");
      const before = cursorOf(terminal);

      await writeTo(terminal, `${reset}X`);

      expect(terminal.buffer.active.type).toBe("normal");
      const cell = terminal.buffer.active.getLine(before.y)?.getCell(before.x);
      expect(cell?.getChars()).toBe("X");
      expect(cell?.isFgDefault()).toBe(true);
      expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe("line1");
    } finally {
      terminal.dispose();
    }
  });

  it("leaves the alternate screen and returns to where the program started", async () => {
    const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    try {
      await writeTo(terminal, "line1\r\nline2\r\n$ htop");
      const before = cursorOf(terminal);
      await writeTo(terminal, "\u001b[?1049h\u001b[?1000h\u001b[?1006h\u001b[10;20HTUI\u001b7\u001b[3;3H");
      expect(terminal.buffer.active.type).toBe("alternate");

      await writeTo(terminal, reset);

      expect(terminal.buffer.active.type).toBe("normal");
      expect(terminal.modes.mouseTrackingMode).toBe("none");
      expect(cursorOf(terminal)).toEqual(before);
      expect(terminal.buffer.active.getLine(1)?.translateToString(true)).toBe("line2");
    } finally {
      terminal.dispose();
    }
  });

  it("stops encoding keys with the kitty keyboard protocol", async () => {
    const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    const kittyKeyboard = attachKittyKeyboardProtocol(terminal.parser);
    try {
      await writeTo(terminal, "\u001b[>1u\u001b[>3u");
      const controlC = new KeyboardEvent("keydown", { key: "c", ctrlKey: true });
      expect(kittyKeyboard.encode(controlC)).toBe("\u001b[99;5u");

      await writeTo(terminal, reset);

      expect(kittyKeyboard.encode(controlC)).toBeNull();
      // 終わったプログラムが積んだフラグも残さない。次のシェルでpushしていないプログラムが
      // popしても、前のフラグへ戻らない。
      await writeTo(terminal, "\u001b[<u");
      expect(kittyKeyboard.encode(controlC)).toBeNull();
    } finally {
      kittyKeyboard.dispose();
      terminal.dispose();
    }
  });
});

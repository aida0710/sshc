import { Terminal, type IBuffer } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import fixtures from "../../../internal/terminal/testdata/screen-snapshots.json";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";

// The engine follows each session's output with a terminal emulator of its own and, for a terminal that cannot
// replay the output from the start, sends that emulator's screen and then the output it still holds
// (internal/terminal/screen_checkpoint.go). Each fixture is written out by the engine
// (internal/terminal/screen_snapshot_fixture_test.go): xterm.js given the screen and the output after it must show
// what xterm.js given the whole output shows.

function terminalLike(cols: number, rows: number): Terminal {
  // The options of createTerminalView.ts that change how output is read.
  const terminal = new Terminal({ cols, rows, allowProposedApi: true, scrollOnEraseInDisplay: true, vtExtensions: { kittyKeyboard: true } });
  applyCurrentUnicodeWidths(terminal);
  return terminal;
}

function writeTo(terminal: Terminal, output: string): Promise<void> {
  return new Promise((resolve) => terminal.write(output, resolve));
}

// shownRows is what a buffer shows: its rows from the top of the screen, with each cell's character and look.
function shownRows(buffer: IBuffer, rows: number) {
  const shown = [];
  for (let y = 0; y < rows; y++) {
    const line = buffer.getLine(buffer.baseY + y);
    const cells = [];
    for (let x = 0; line !== undefined && x < line.length; x++) {
      const cell = line.getCell(x);
      if (cell === undefined) continue;
      // A space shows as a cell never written to (erased, with or without a background colour): the engine leaves
      // a blank row's end out and writes the others as spaces.
      const chars = cell.getChars() === "" && cell.getWidth() === 1 ? " " : cell.getChars();
      if (chars === " " && cell.isAttributeDefault()) continue;
      cells.push({
        x,
        chars,
        width: cell.getWidth(),
        fg: [cell.getFgColorMode(), cell.getFgColor()],
        bg: [cell.getBgColorMode(), cell.getBgColor()],
        bold: cell.isBold(),
        italic: cell.isItalic(),
        underline: cell.isUnderline(),
        inverse: cell.isInverse(),
      });
    }
    // The engine's emulator keeps a coloured space and a cell erased in that colour alike, so the text is compared
    // without the spaces it ends with; the cells above still say what each shows.
    shown.push({ text: (line?.translateToString(true) ?? "").trimEnd(), cells });
  }
  return shown;
}

function stateOf(terminal: Terminal) {
  const active = terminal.buffer.active;
  return {
    screen: active.type,
    cursor: { x: active.cursorX, y: active.cursorY },
    modes: terminal.modes,
    active: shownRows(active, terminal.rows),
    // The normal screen comes back when a program leaves the alternate one.
    normal: shownRows(terminal.buffer.normal, terminal.rows),
  };
}

// kittyFlags asks the terminal for its kitty keyboard flags (CSI ? u) and reads the answer.
async function kittyFlags(terminal: Terminal): Promise<string> {
  let answer = "";
  const listener = terminal.onData((data) => { answer += data; });
  await writeTo(terminal, "\x1b[?u");
  listener.dispose();
  return answer;
}

describe("the screen the engine sends for a terminal that cannot replay from the start", () => {
  for (const fixture of fixtures) {
    it(`shows what the whole output shows: ${fixture.name}`, async () => {
      const whole = terminalLike(fixture.cols, fixture.rows);
      const rebuilt = terminalLike(fixture.cols, fixture.rows);
      try {
        await writeTo(whole, fixture.output);
        await writeTo(rebuilt, fixture.screen);
        await writeTo(rebuilt, fixture.tail);
        expect(stateOf(rebuilt)).toEqual(stateOf(whole));
        expect(await kittyFlags(rebuilt)).toBe(await kittyFlags(whole));
      } finally {
        whole.dispose();
        rebuilt.dispose();
      }
    });
  }

  it("brings a terminal that showed something else to the same screen", async () => {
    const fixture = fixtures[0]!;
    const whole = terminalLike(fixture.cols, fixture.rows);
    const resumed = terminalLike(fixture.cols, fixture.rows);
    try {
      await writeTo(whole, fixture.output);
      await writeTo(resumed, "\x1b[?1049h\x1b[?1003h\x1b[31mleft over\x1b[5;10r\x1b[>4u");
      await writeTo(resumed, fixture.screen);
      await writeTo(resumed, fixture.tail);
      expect(stateOf(resumed)).toEqual(stateOf(whole));
    } finally {
      whole.dispose();
      resumed.dispose();
    }
  });
});

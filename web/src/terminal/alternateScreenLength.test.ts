import { Terminal } from "@xterm/xterm";
import { describe, expect, test } from "vitest";
import { fitEmptyAlternateScreen } from "./alternateScreenLength";

const tallRows = 36;
const shortRows = 19;
const cols = 41;

function write(view: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => view.write(data, resolve));
}

// redraw is a program on the alternate screen drawing every row at the terminal's size, as vim does on SIGWINCH.
function redraw(rows: number): string {
  let drawing = "\x1b[H\x1b[2J";
  for (let row = 1; row <= rows; row += 1) drawing += `\x1b[${row};1Hline ${row}`;
  return drawing;
}

function shownRows(view: Terminal): string[] {
  const buffer = view.buffer.active;
  return Array.from({ length: view.rows }, (_, row) => buffer.getLine(buffer.viewportY + row)?.translateToString(true) ?? "");
}

// The order on a phone: a program leaves the alternate screen with the keyboard down, the keyboard comes up, the next
// program enters the alternate screen, its first output runs past the bottom row and it puts the cursor back at the
// top (vim does all of this), and the keyboard goes down again.
async function leaveShortenEnterGrow(view: Terminal): Promise<string[]> {
  await write(view, "\x1b[?1049h\x1b[?1049l");
  view.resize(cols, shortRows);
  await write(view, "\x1b[?1049h");
  for (let line = 0; line < shortRows + 3; line += 1) await write(view, `first output ${line}\r\n`);
  await write(view, "\x1b[H");
  view.resize(cols, tallRows);
  await write(view, redraw(tallRows));
  return shownRows(view);
}

describe("fitEmptyAlternateScreen", () => {
  test("a program that enters the alternate screen after the terminal got shorter is drawn in place once it is taller", async () => {
    const view = new Terminal({ cols, rows: tallRows, allowProposedApi: true });
    const fitting = fitEmptyAlternateScreen(view);

    const rows = await leaveShortenEnterGrow(view);

    const screen = view.buffer.active;
    expect(screen.type).toBe("alternate");
    expect(screen.baseY).toBe(0);
    expect(screen.baseY + view.rows).toBeLessThanOrEqual(screen.length);
    expect(rows[0]).toBe("line 1");
    expect(rows[tallRows - 1]).toBe(`line ${tallRows}`);
    fitting.dispose();
    view.dispose();
  });

  // When this fails, xterm.js lowers an empty alternate screen's limit itself: fitEmptyAlternateScreen and these
  // tests can go.
  test("xterm.js on its own still lets that alternate screen scroll, and its screen then runs past the buffer's end", async () => {
    const view = new Terminal({ cols, rows: tallRows, allowProposedApi: true });

    await leaveShortenEnterGrow(view);

    const screen = view.buffer.active;
    expect(screen.baseY).toBeGreaterThan(0);
    expect(screen.baseY + view.rows).toBeGreaterThan(screen.length);
    view.dispose();
  });
});

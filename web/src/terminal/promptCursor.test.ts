import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { trackPromptCursor } from "./promptCursor";

const prompt = "\u001b]133;A\u0007$ \u001b]133;B\u0007";
const left = "\u001b[D";
const right = "\u001b[C";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

function shell() {
  const view = new Terminal({ cols: 40, rows: 6, allowProposedApi: true });
  const cursor = trackPromptCursor(view);
  cleanups.push(() => {
    cursor.dispose();
    view.dispose();
  });
  const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));
  return { view, cursor, write };
}

describe("moving the cursor by clicking at a prompt", () => {
  it("sends one arrow key for each character, counting a wide character once", async () => {
    const { cursor, write } = shell();
    // "$ " is the prompt; the typed text starts at column 2 and 日本 take two cells each.
    await write(`${prompt}echo 日本abc`);

    // The right half of 本 (column 10) is the character itself.
    expect(cursor.arrowsTo({ x: 10, y: 0 })).toBe(left.repeat(4));
    expect(cursor.arrowsTo({ x: 2, y: 0 })).toBe(left.repeat(10));
  });

  it("stops at the end of the typed text and moves right from inside it", async () => {
    const { cursor, write } = shell();
    await write(`${prompt}echo abc\u001b[3D`);

    expect(cursor.arrowsTo({ x: 30, y: 0 })).toBe(right.repeat(3));
    expect(cursor.arrowsTo({ x: 7, y: 0 })).toBeNull();
  });

  it("sends the keys a program in application cursor mode expects", async () => {
    const { cursor, write } = shell();
    await write(`${prompt}ls\u001b[?1h`);

    expect(cursor.arrowsTo({ x: 2, y: 0 })).toBe("\u001bOD".repeat(2));
  });

  it("leaves a click alone outside the text being typed at a prompt", async () => {
    const { cursor, write } = shell();
    await write("no shell integration");
    expect(cursor.arrowsTo({ x: 3, y: 0 })).toBeNull();

    await write(`\r\n${prompt}make`);
    // The prompt itself and another line.
    expect(cursor.arrowsTo({ x: 0, y: 1 })).toBeNull();
    expect(cursor.arrowsTo({ x: 3, y: 0 })).toBeNull();

    // A program that reads the mouse gets the click.
    await write("\u001b[?1000h");
    expect(cursor.arrowsTo({ x: 2, y: 1 })).toBeNull();
    await write("\u001b[?1000l");
    expect(cursor.arrowsTo({ x: 2, y: 1 })).toBe(left.repeat(4));

    // Once the command runs, the line is no longer being typed.
    await write("\u001b]133;C\u0007");
    expect(cursor.arrowsTo({ x: 2, y: 1 })).toBeNull();
  });

  it("leaves typed text that wrapped onto the next line alone", async () => {
    const { cursor, write } = shell();
    await write(`${prompt}${"x".repeat(45)}`);

    expect(cursor.arrowsTo({ x: 5, y: 0 })).toBeNull();
    expect(cursor.arrowsTo({ x: 2, y: 1 })).toBeNull();
  });
});

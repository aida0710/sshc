import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { blockAtTop, lastLineOf, outputText, trackCommandBlocks } from "./commandBlocks";

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

const A = "\u001b]133;A\u0007";
const B = "\u001b]133;B\u0007";
const C = "\u001b]133;C\u0007";
const D = (code?: number) => `\u001b]133;D${code === undefined ? "" : `;${code}`}\u0007`;
const time = (kind: "C" | "D", at: number) => `\u001b]7742;${kind};${at}\u0007`;

function harness(rows = 10) {
  const view = new Terminal({ cols: 40, rows, allowProposedApi: true });
  let directory = "/home/aida/web";
  const tracked = trackCommandBlocks(view, { directory: () => directory });
  let changes = 0;
  tracked.onChange(() => { changes += 1; });
  cleanups.push(() => {
    tracked.dispose();
    view.dispose();
  });
  const write = (data: string) => new Promise<void>((resolve) => view.write(data, resolve));
  return { view, tracked, write, changes: () => changes, cd: (path: string) => { directory = path; } };
}

describe("trackCommandBlocks", () => {
  it("follows a command from its prompt to its end, with the engine's times and the exit code", async () => {
    const { view, tracked, write, changes } = harness();
    await write(`${A}$ ${B}make test\r\n${C}${time("C", 1000)}building\r\nfailed\r\n${D(2)}${time("D", 5200)}${A}$ ${B}`);

    const [block] = tracked.blocks();
    expect(tracked.blocks()).toHaveLength(1);
    expect(block).toMatchObject({ command: "make test", directory: "/home/aida/web", status: "failed", exitCode: 2, startedAt: 1000, finishedAt: 5200 });
    expect(block!.start.line).toBe(0);
    expect(block!.output.line).toBe(1);
    expect(lastLineOf(block!, view.buffer.active)).toBe(2);
    expect(outputText(block!, view.buffer.active)).toBe("building\nfailed");
    expect(changes()).toBe(4);
  });

  it("keeps a running command open down to the cursor, and tells success from an end without a code", async () => {
    const { view, tracked, write, cd } = harness();
    await write(`${A}$ ${B}true\r\n${C}${D(0)}${A}$ ${B}echo\r\n${C}${D()}`);
    cd("/tmp");
    await write(`${A}$ ${B}sleep 9\r\n${C}one\r\ntwo`);
    const [succeeded, finished, running] = tracked.blocks();
    expect(succeeded?.status).toBe("succeeded");
    expect(finished?.status).toBe("finished");
    expect(running).toMatchObject({ command: "sleep 9", directory: "/tmp", status: "running", end: null });
    expect(lastLineOf(running!, view.buffer.active)).toBe(view.buffer.active.baseY + view.buffer.active.cursorY);
  });

  it("joins a command the terminal wrapped onto the next line", async () => {
    const { tracked, write } = harness();
    const long = `echo ${"x".repeat(50)}`;
    await write(`${A}$ ${B}${long}\r\n${C}`);
    expect(tracked.blocks()[0]?.command).toBe(long);
  });

  it("takes the line before the output as the prompt for a shell that sends only C and D", async () => {
    const { tracked, write } = harness();
    await write(`$ ls\r\n${C}a.txt\r\n${D(0)}$ `);
    const [block] = tracked.blocks();
    expect(block).toMatchObject({ command: "", status: "succeeded" });
    expect(block!.start.line).toBe(0);
  });

  it("names the command whose output fills the top of the view while its prompt is above it", async () => {
    const { view, tracked, write } = harness(5);
    const lines = Array.from({ length: 12 }, (_, index) => `line ${index}`).join("\r\n");
    await write(`${A}$ ${B}seq 12\r\n${C}${lines}\r\n${D(0)}${A}$ ${B}`);
    const [block] = tracked.blocks();
    view.scrollToLine(0);
    expect(blockAtTop(tracked.blocks(), view.buffer.active)).toBeNull();
    view.scrollToLine(4);
    expect(blockAtTop(tracked.blocks(), view.buffer.active)).toBe(block);
    // The next command's prompt on the top line: nothing to repeat above it.
    await write(`seq 9\r\n${C}${lines}\r\n${D(0)}${A}$ ${B}`);
    const next = tracked.blocks()[1]!;
    view.scrollToLine(next.start.line);
    expect(view.buffer.active.viewportY).toBe(next.start.line);
    expect(blockAtTop(tracked.blocks(), view.buffer.active)).toBeNull();
  });
});

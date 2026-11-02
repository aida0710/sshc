import { Terminal } from "@xterm/xterm";
import { describe, expect, test, vi } from "vitest";
import { resizeRewritingCursorLine } from "./cursorLineRewrite";

const question = "The authenticity of host 'web-01 (10.0.2.15)' cannot be established.\r\n"
  + "Are you sure you want to continue connecting (yes/no)? ";

function write(view: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => view.write(data, resolve));
}

function lines(view: Terminal): string[] {
  const buffer = view.buffer.active;
  return Array.from({ length: buffer.length }, (_, row) => buffer.getLine(row)?.translateToString(true) ?? "").filter((line) => line !== "");
}

async function resized(view: Terminal, cols: number): Promise<void> {
  resizeRewritingCursorLine(view, () => view.resize(cols, view.rows));
  await write(view, "");
}

describe("resizeRewritingCursorLine", () => {
  test("xterm.js alone cuts off a question written wider than the terminal becomes", async () => {
    // The reason for the rewrite: if this ever passes, xterm.js reflows the cursor's line by itself.
    const view = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    await write(view, question);
    view.resize(40, 24);
    await write(view, "");
    expect(lines(view).at(-1)).toBe("Are you sure you want to continue connec");
  });

  test("writes a question again at a narrower width, wrapped, with the cursor after it", async () => {
    const view = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    await write(view, question);
    await resized(view, 40);
    expect(lines(view).slice(-2)).toEqual(["Are you sure you want to continue connec", "ting (yes/no)? "]);
    await write(view, "yes");
    expect(lines(view).at(-1)).toBe("ting (yes/no)? yes");
    // The lines above, which xterm.js reflows itself, are left alone.
    expect(lines(view).slice(0, 2)).toEqual(["The authenticity of host 'web-01 (10.0.2", ".15)' cannot be established."]);
  });

  test("joins a question wrapped at a narrow width again when the terminal gets wider", async () => {
    const view = new Terminal({ cols: 40, rows: 24, allowProposedApi: true });
    await write(view, question);
    await resized(view, 80);
    expect(lines(view).at(-1)).toBe("Are you sure you want to continue connecting (yes/no)? ");
    await write(view, "no");
    expect(lines(view).at(-1)).toBe("Are you sure you want to continue connecting (yes/no)? no");
  });

  test("writes nothing when the cursor starts an empty line or the width stays", async () => {
    const view = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    await write(view, "[sshc] connecting\r\n");
    const written = vi.spyOn(view, "write");
    resizeRewritingCursorLine(view, () => view.resize(40, 24));
    resizeRewritingCursorLine(view, () => view.resize(40, 20));
    expect(written).not.toHaveBeenCalled();
  });
});

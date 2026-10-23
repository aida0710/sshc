import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { imagePathsInBuffer } from "./imagePaths";

async function terminalWithOutput(output: string, cols = 30): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: 24, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

describe("imagePathsInBuffer", () => {
  it("lists image paths oldest first, each once, and passes other paths by", async () => {
    const terminal = await terminalWithOutput([
      "[image] /tmp/a.png (1 KB)",
      "ran /usr/bin/env",
      "[image] /tmp/b.JPG (2 KB)",
      "[image] /tmp/a.png (1 KB)",
    ].join("\r\n"), 80);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active)).toEqual(["/tmp/a.png", "/tmp/b.JPG"]);
    } finally { terminal.dispose(); }
  });

  it("finds a path that wraps, including one that starts on a wrapped row, once", async () => {
    const path = "/home/someone/projects/screenshots/very-long-name.png";
    const later = "/tmp/later.png";
    const terminal = await terminalWithOutput(`[image] ${path} then ${later}`);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active)).toEqual([path, later]);
    } finally { terminal.dispose(); }
  });
});

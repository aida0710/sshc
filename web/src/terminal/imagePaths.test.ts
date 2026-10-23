import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { imagePathsInBuffer } from "./imagePaths";

async function terminalWithOutput(output: string, cols = 30): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: 24, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

describe("imagePathsInBuffer", () => {
  it("resolves wrapped home paths separately from cwd and removes duplicates after resolution", async () => {
    const terminal = await terminalWithOutput([
      "[image] ~/projects/long-folder/a.png",
      "[image] /users/alex/projects/long-folder/a.png",
      "[image] shots/b.png",
      "[image] ~/shots/c.jpg",
      "~/notes.txt ~other/d.png",
    ].join("\r\n"));
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, "/srv/project", "/users/alex")).toEqual([
        "/users/alex/projects/long-folder/a.png", "/srv/project/shots/b.png", "/users/alex/shots/c.jpg",
      ]);
      expect(imagePathsInBuffer(terminal.buffer.active, null, "/users/alex")).toEqual([
        "/users/alex/projects/long-folder/a.png", "/users/alex/shots/c.jpg",
      ]);
      expect(imagePathsInBuffer(terminal.buffer.active, "/srv/project", null)).toEqual([
        "/users/alex/projects/long-folder/a.png", "/srv/project/shots/b.png",
      ]);
    } finally { terminal.dispose(); }
  });

  it("lists image paths oldest first, each once, and passes other paths by", async () => {
    const terminal = await terminalWithOutput([
      "[image] /tmp/a.png (1 KB)",
      "ran /usr/bin/env",
      "[image] /tmp/b.JPG (2 KB)",
      "[image] /tmp/a.png (1 KB)",
    ].join("\r\n"), 80);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, null, null)).toEqual(["/tmp/a.png", "/tmp/b.JPG"]);
    } finally { terminal.dispose(); }
  });

  it("finds a path that wraps, including one that starts on a wrapped row, once", async () => {
    const path = "/home/someone/projects/screenshots/very-long-name.png";
    const later = "/tmp/later.png";
    const terminal = await terminalWithOutput(`[image] ${path} then ${later}`);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, null, null)).toEqual([path, later]);
    } finally { terminal.dispose(); }
  });

  it("resolves relative image paths against the directory, and leaves them out without one", async () => {
    const terminal = await terminalWithOutput("[image] shots/a.png (1 KB)\r\n[image] /tmp/b.png (1 KB)", 80);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, "/srv", null)).toEqual(["/srv/shots/a.png", "/tmp/b.png"]);
      expect(imagePathsInBuffer(terminal.buffer.active, null, null)).toEqual(["/tmp/b.png"]);
    } finally { terminal.dispose(); }
  });
});

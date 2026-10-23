import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { imagePathsInBuffer } from "./imagePaths";

async function terminalWithOutput(output: string, cols = 30): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: 24, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

describe("imagePathsInBuffer", () => {
  it("shares Unicode, quoting, escapes and URI decoding with clickable links across wrapped rows", async () => {
    const terminal = await terminalWithOutput([
      '→/tmp/画面.png', '"/tmp/画面 2.png"', '/tmp/画面\\ 2.png',
      'file:///tmp/%E7%94%BB%E9%9D%A2%202.png',
      'artifacts/😀.png', '$HOME/画像/a.png', '${HOME}/画像/b.png',
      '~/projects/a b/x.png', '/tmp/a\\ b.png',
    ].join('\r\n'), 19);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: '/work', home: '/home/aida' })).toEqual([
        '/tmp/画面.png', '/tmp/画面 2.png', '/work/artifacts/😀.png',
        '/home/aida/画像/a.png', '/home/aida/画像/b.png', '/tmp/a b.png',
      ]);
    } finally { terminal.dispose(); }
  });

  it("includes Windows images only when the same local preview support is enabled", async () => {
    const terminal = await terminalWithOutput('C:\\Users\\aida\\a.png\r\nC:/Users/aida/a.png\r\n"C:\\Users\\aida\\b c.png"');
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: null })).toEqual([]);
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: null, allowWindowsPaths: true })).toEqual([
        'C:/Users/aida/a.png', 'C:/Users/aida/b c.png',
      ]);
    } finally { terminal.dispose(); }
  });

  it("resolves wrapped home paths separately from cwd and removes duplicates after resolution", async () => {
    const terminal = await terminalWithOutput([
      "[image] ~/projects/long-folder/a.png",
      "[image] /users/alex/projects/long-folder/a.png",
      "[image] shots/b.png",
      "[image] ~/shots/c.jpg",
      "~/notes.txt ~other/d.png",
    ].join("\r\n"));
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: "/srv/project", home: "/users/alex" })).toEqual([
        "/users/alex/projects/long-folder/a.png", "/srv/project/shots/b.png", "/users/alex/shots/c.jpg",
      ]);
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: "/users/alex" })).toEqual([
        "/users/alex/projects/long-folder/a.png", "/users/alex/shots/c.jpg",
      ]);
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: "/srv/project", home: null })).toEqual([
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
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: null })).toEqual(["/tmp/a.png", "/tmp/b.JPG"]);
    } finally { terminal.dispose(); }
  });

  it("finds a path that wraps, including one that starts on a wrapped row, once", async () => {
    const path = "/home/someone/projects/screenshots/very-long-name.png";
    const later = "/tmp/later.png";
    const terminal = await terminalWithOutput(`[image] ${path} then ${later}`);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: null })).toEqual([path, later]);
    } finally { terminal.dispose(); }
  });

  it("resolves relative image paths against the directory, and leaves them out without one", async () => {
    const terminal = await terminalWithOutput("[image] shots/a.png (1 KB)\r\n[image] /tmp/b.png (1 KB)", 80);
    try {
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: "/srv", home: null })).toEqual(["/srv/shots/a.png", "/tmp/b.png"]);
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: null, home: null })).toEqual(["/tmp/b.png"]);
    } finally { terminal.dispose(); }
  });
});

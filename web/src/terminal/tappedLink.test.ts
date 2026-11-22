import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { linkAtPoint } from "./tappedLink";

const cellWidth = 10;
const cellHeight = 20;

async function tappable(output: string, cols = 30, rows = 4) {
  const terminal = new Terminal({ cols, rows, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  const element = document.createElement("div");
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  screen.getBoundingClientRect = () => new DOMRect(100, 50, cols * cellWidth, rows * cellHeight);
  element.appendChild(screen);
  // _core は xterm.js の中身で、OSC 8 のリンクの宛先はここから読む。
  const view = { element, rows, cols, buffer: terminal.buffer, options: terminal.options, _core: (terminal as unknown as { _core: unknown })._core };
  return { terminal, view };
}

// The point in the middle of a 1-based cell.
function cell(column: number, row: number) {
  return { clientX: 100 + (column - 0.5) * cellWidth, clientY: 50 + (row - 0.5) * cellHeight };
}

describe("linkAtPoint", () => {
  it("Windowsのパスはローカル読み取りを許可したときだけ押せる", async () => {
    const { terminal, view } = await tappable("C:/Users/aida/a.png");
    try {
      expect(linkAtPoint(view, cell(10, 1))).toBeNull();
      expect(linkAtPoint(view, cell(10, 1), true)).toMatchObject({ kind: "path", target: "C:/Users/aida/a.png" });
    } finally { terminal.dispose(); }
  });

  it("URLを押すとそのURLを返す", async () => {
    const { terminal, view } = await tappable("see https://example.test/a now");
    try {
      expect(linkAtPoint(view, cell(10, 1))).toMatchObject({ kind: "url", target: "https://example.test/a" });
    } finally { terminal.dispose(); }
  });

  it("URLの外を押しても何も返さない", async () => {
    const { terminal, view } = await tappable("see https://example.test/a now");
    try {
      expect(linkAtPoint(view, cell(2, 1))).toBeNull();
      expect(linkAtPoint(view, cell(28, 1))).toBeNull();
      expect(linkAtPoint(view, cell(10, 2))).toBeNull();
    } finally { terminal.dispose(); }
  });

  it("折り返したURLは次の行を押しても同じURLを返す", async () => {
    const url = "https://example.test/a-long-path-that-wraps";
    const { terminal, view } = await tappable(url);
    try {
      expect(linkAtPoint(view, cell(3, 2))).toMatchObject({ target: url });
    } finally { terminal.dispose(); }
  });

  it("プログラムが折り返したパスは各行で押せて字下げは押せない", async () => {
    const { terminal, view } = await tappable("/tmp/123456789012345\x1b[K\r\x1b[1B    image.png", 20);
    try {
      const target = "/tmp/123456789012345image.png";
      expect(linkAtPoint(view, cell(20, 1))).toMatchObject({ target });
      expect(linkAtPoint(view, cell(5, 2))).toMatchObject({ target });
      expect(linkAtPoint(view, cell(13, 2))).toMatchObject({ target });
      expect(linkAtPoint(view, cell(4, 2))).toBeNull();
      expect(linkAtPoint(view, cell(14, 2))).toBeNull();
    } finally { terminal.dispose(); }
  });

  it("画面の外を押しても何も返さない", async () => {
    const { terminal, view } = await tappable("https://example.test/a");
    try {
      expect(linkAtPoint(view, { clientX: 90, clientY: 60 })).toBeNull();
      expect(linkAtPoint(view, cell(5, 5))).toBeNull();
    } finally { terminal.dispose(); }
  });
  describe("プログラムが OSC 8 でリンクを付けた文字", () => {
    const hardBreak = "\x1b[K\r\x1b[1B";
    const linked = (target: string, text: string, id = "a") => `\x1b]8;id=${id};${target}\x07${text}\x1b]8;;\x07`;

    it("続きの行に / や = が無い URL も、どの行を押しても宛先の全体を返す", async () => {
      const url = "https://example.test/d/1a9Nex-6jpYi_NXIHhtEOiYgzHyjgA0qx7hU7sims5tg";
      const output = `see (${linked(url, url.slice(0, 25))}${hardBreak}  ${linked(url, url.slice(25))}) for it`;
      const { terminal, view } = await tappable(output, 30);
      try {
        expect(linkAtPoint(view, cell(10, 1))).toMatchObject({ kind: "url", target: url, text: url });
        expect(linkAtPoint(view, cell(10, 2))).toMatchObject({ kind: "url", target: url, text: url });
      } finally { terminal.dispose(); }
    });

    it("文字が URL でないリンクは、宛先と画面の文字を分けて返す", async () => {
      const { terminal, view } = await tappable(`read ${linked("https://example.test/actual", "the guide")} first`);
      try {
        expect(linkAtPoint(view, cell(8, 1))).toMatchObject({ kind: "url", target: "https://example.test/actual", text: "the guide", start: 5, end: 14 });
      } finally { terminal.dispose(); }
    });

    it("リンクの外の文字を押しても何も返さない", async () => {
      const { terminal, view } = await tappable(`read ${linked("https://example.test/actual", "the guide")} first`);
      try {
        expect(linkAtPoint(view, cell(2, 1))).toBeNull();
        expect(linkAtPoint(view, cell(17, 1))).toBeNull();
      } finally { terminal.dispose(); }
    });

    it("http と https でない宛先のリンクは返さない", async () => {
      const { terminal, view } = await tappable(linked("javascript:alert(1)", "press here"));
      try {
        expect(linkAtPoint(view, cell(3, 1))).toBeNull();
      } finally { terminal.dispose(); }
    });
  });
});

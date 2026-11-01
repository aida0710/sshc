import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { bufferRangeText, findBufferLinks } from "./bufferLinks";
import { attachLinkProvider } from "./linkProvider";
import type { ILinkProvider, ILink } from "@xterm/xterm";

async function terminalWithOutput(output: string, cols = 30): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: 24, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

describe("wrapped terminal links", () => {
  const url = "https://example.test/authorize?redirect_uri=http%3A%2F%2F127.0.0.1&state=abc123";

  it("recognizes the whole URL from every wrapped row", async () => {
    const terminal = await terminalWithOutput(url);
    try {
      const expected = {
        match: { kind: "url", text: url, target: url, start: 0, end: url.length },
        range: { start: { x: 1, y: 1 }, end: { x: url.length % 30, y: 3 } },
      };
      for (const row of [1, 2, 3]) {
        expect(findBufferLinks(terminal.buffer.active, row)).toEqual([expected]);
      }
    } finally { terminal.dispose(); }
  });

  it("counts Japanese and emoji prefixes by terminal cells", async () => {
    const terminal = await terminalWithOutput(`説明😀 ${url}`);
    try {
      expect(findBufferLinks(terminal.buffer.active, 2)[0]).toMatchObject({
        match: { target: url },
        // xterm's default Unicode 6 table renders this emoji in one cell.
        range: { start: { x: 7, y: 1 }, end: { x: (6 + url.length) % 30, y: 3 } },
      });
    } finally { terminal.dispose(); }
  });

  it("does not join explicit newlines or unrelated rows", async () => {
    const terminal = await terminalWithOutput("https://example.test/\r\nseparate\r\nplain text");
    try {
      expect(findBufferLinks(terminal.buffer.active, 1)[0]?.match.target).toBe("https://example.test/");
      expect(findBufferLinks(terminal.buffer.active, 2)).toEqual([]);
      expect(findBufferLinks(terminal.buffer.active, 3)).toEqual([]);
    } finally { terminal.dispose(); }
  });

  it("keeps a wide URL character when xterm pads the previous row", async () => {
    const target = "https://a.test/aaaa界/end";
    const terminal = await terminalWithOutput(target, 20);
    try {
      expect(findBufferLinks(terminal.buffer.active, 2)[0]?.match.target).toBe(target);
    } finally { terminal.dispose(); }
  });

  it("keeps the complete URL after the terminal is resized", async () => {
    const terminal = await terminalWithOutput(`${url}\r\n`);
    try {
      terminal.resize(25, 24);
      expect(findBufferLinks(terminal.buffer.active, 3)[0]).toMatchObject({
        match: { target: url },
        range: { start: { x: 1, y: 1 }, end: { x: url.length % 25, y: 4 } },
      });
    } finally { terminal.dispose(); }
  });

  it("opens or selects the full URL when clicked on a continuation row", async () => {
    const terminal = await terminalWithOutput(url);
    let provider: ILinkProvider | undefined;
    terminal.registerLinkProvider = (registered) => { provider = registered; return { dispose() {} }; };
    const opened: string[] = [];
    const selected: string[] = [];
    attachLinkProvider(terminal, { open: (target) => opened.push(target), select: (link) => selected.push(link.target), point: () => {} });
    try {
      const links = await new Promise<ILink[] | undefined>((resolve) => provider?.provideLinks(2, resolve));
      expect(links).toHaveLength(1);
      links![0]!.activate(new MouseEvent("click", { metaKey: true }), url);
      links![0]!.activate(new MouseEvent("click"), url);
      expect(opened).toEqual([url]);
      expect(selected).toEqual([url]);
    } finally { terminal.dispose(); }
  });
});

// The ranges below are the ones xterm.js gives an OSC 8 link: 1-based, the end
// column inclusive, continued across wrapped rows.
describe("the text a link range covers", () => {
  it("reads the label by cells after Japanese text", async () => {
    const terminal = await terminalWithOutput("日本語の説明 https://example.test/", 40);
    try {
      // 日本語の説明 takes 12 cells and the space one more.
      expect(bufferRangeText(terminal.buffer.active, { start: { x: 14, y: 1 }, end: { x: 34, y: 1 } }))
        .toBe("https://example.test/");
    } finally { terminal.dispose(); }
  });

  it("reads the label after a combining character", async () => {
    const terminal = await terminalWithOutput("e\u0301 docs");
    try {
      expect(bufferRangeText(terminal.buffer.active, { start: { x: 3, y: 1 }, end: { x: 6, y: 1 } })).toBe("docs");
    } finally { terminal.dispose(); }
  });

  it("joins a label that wraps onto the next row", async () => {
    const terminal = await terminalWithOutput("see https://example.test/a-long-path", 20);
    try {
      expect(bufferRangeText(terminal.buffer.active, { start: { x: 5, y: 1 }, end: { x: 16, y: 2 } }))
        .toBe("https://example.test/a-long-path");
    } finally { terminal.dispose(); }
  });
});

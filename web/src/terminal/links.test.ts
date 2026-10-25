import { describe, expect, it } from "vitest";
import { findTerminalLinks, linkTextNamesTarget, modifierOpensLink, osc8Link } from "./links";

describe("findTerminalLinks", () => {
  it.each([
    ['[image] /home/aida/スクショ/画面.png', 'path', '/home/aida/スクショ/画面.png'],
    ['[image] ~/projects/a b/x.png', null, null],
    ['saved "/tmp/a b.png"', 'path', '/tmp/a b.png'],
    ["saved '/tmp/x.png'", 'path', '/tmp/x.png'],
    ['saved `/tmp/x.png`', 'path', '/tmp/x.png'],
    ['Saved to:/tmp/x.png', 'path', '/tmp/x.png'],
    ['保存しました：/tmp/x.png', 'path', '/tmp/x.png'],
    ['→/tmp/x.png', 'path', '/tmp/x.png'],
    ['➜/tmp/x.png', 'path', '/tmp/x.png'],
    ['file:///tmp/x.png', 'path', '/tmp/x.png'],
    ['file:///tmp/x.png:12:3', 'path', '/tmp/x.png'],
    ['file:///tmp/a%3A12.png', 'path', '/tmp/a:12.png'],
    ['C:\\Users\\aida\\x.png', null, null],
    ['C:/Users/aida/x.png', null, null],
    ['$HOME/x.png', 'relative-path', '~/x.png'],
    ['~/x.PNG', 'relative-path', '~/x.PNG'],
    ['~/x.svg', 'relative-path', '~/x.svg'],
    ['screenshot.png', 'relative-path', 'screenshot.png'],
    ['./out/x.jpeg', 'relative-path', './out/x.jpeg'],
    ['../x.webp', 'relative-path', '../x.webp'],
    ['/tmp/x.png:12', 'path', '/tmp/x.png'],
    ['/tmp/a:b/x.png:12:4', 'path', '/tmp/a:b/x.png'],
    ['/tmp/a\\ b.png', 'path', '/tmp/a b.png'],
    ['artifacts/画面.png', 'relative-path', 'artifacts/画面.png'],
    ['artifacts/📁/画面.png', 'relative-path', 'artifacts/📁/画面.png'],
    ['📁/画面.png', 'relative-path', '📁/画面.png'],
    ['artifacts/~/画面.png', 'relative-path', 'artifacts/~/画面.png'],
    ['${HOME}/画面.png', 'relative-path', '~/画面.png'],
    ['"${HOME}/a b.png"', 'relative-path', '~/a b.png'],
    ["'artifacts/画面 1.png'", 'relative-path', 'artifacts/画面 1.png'],
    ['file:///tmp/%E7%94%BB%E9%9D%A2%201.png', 'path', '/tmp/画面 1.png'],
    ['/tmp/画面.png:12:3。）', 'path', '/tmp/画面.png'],
    ['artifacts/画面.png:12:3、', 'relative-path', 'artifacts/画面.png'],
    ['/tmp/e\u0301/😀.png', 'path', '/tmp/e\u0301/😀.png'],
  ])('detects the complete path in %s', (line, kind, target) => {
    const links = findTerminalLinks(line);
    expect(links.map(({ kind, target }) => ({ kind, target }))).toEqual(kind === null ? [] : [{ kind, target }]);
    for (const link of links) expect(line.slice(link.start, link.end)).toBe(link.text);
  });

  it.each([
    '/tmp/a b.png', '/tmp/a b.png:12:3', '/tmp/a long name.png', '/tmp/a b/x.png', '/tmp/a b.png。', '/tmp/a b.png,',
    '/tmp/a\\ b c.png', '/tmp/a\\ b c/x.png',
    'artifacts/a b.png', '"/tmp/a b.png', "'/tmp/a b.png", '`/tmp/a b.png',
    'foo:bar.png', 'foo:bar', 'ftp://host/tmp/x.png', 'ssh://host/tmp/x.png',
    'file://host/tmp/x.png', 'file:///tmp/%ZZ.png', 'file:///tmp/%00.png',
    '→C:/Users/aida/x.png', 'Saved to:C:/Users/aida/x.png',
    'file:///C:/Users/aida/x.png', 'file:///tmp/x.png?query',
    '$OTHER/x.png', '${OTHER}/x.png', '~user/画面.png', '~user/~/x.png', '$OTHER/~/x.png', '~/x.png.bak',
    '/tmp/' + 'a'.repeat(4096) + '.png',
  ])('does not link fragments of %s', (line) => {
    expect(findTerminalLinks(line)).toEqual([]);
  });

  it.each(['C:\\Users\\aida\\x.png', 'C:/Users/aida/x.png', 'file:///C:/Users/aida/x.png', '→C:/Users/aida/x.png', 'Saved to:C:/Users/aida/x.png'])('accepts %s only with Windows local preview support', (line) => {
    expect(findTerminalLinks(line, true).map(({ kind, target }) => ({ kind, target }))).toEqual([
      { kind: 'path', target: 'C:/Users/aida/x.png' },
    ]);
  });

  it.each(["C:/Users/a b.png", "C:\\Users\\a b.png"])("does not link unquoted Windows fragments in %s", (line) => {
    expect(findTerminalLinks(line, true)).toEqual([]);
  });

  it("finds http links without swallowing sentence punctuation", () => {
    expect(findTerminalLinks("see https://example.test/a?q=1.")).toEqual([
      { kind: "url", text: "https://example.test/a?q=1", target: "https://example.test/a?q=1", start: 4, end: 30 },
    ]);
  });

  it("preserves the closing bracket of an IPv6 URL", () => {
    expect(findTerminalLinks("see https://[::1],").map(({ kind, target }) => ({ kind, target }))).toEqual([
      { kind: "url", target: "https://[::1]" },
    ]);
  });

  it("finds paths and removes source locations from the target", () => {
    expect(findTerminalLinks("failed: /srv/app/main.go:42:7")).toEqual([
      { kind: "path", text: "/srv/app/main.go:42:7", target: "/srv/app/main.go", start: 8, end: 29 },
    ]);
  });

  it("does not turn URL path segments into paths", () => {
    expect(findTerminalLinks("https://example.test/srv/file")).toHaveLength(1);
  });

  it("finds the path of an image Claude Code sent, without its size", () => {
    expect(findTerminalLinks("› [image] /tmp/shot.png (12 KB)")).toEqual([
      { kind: "path", text: "/tmp/shot.png", target: "/tmp/shot.png", start: 10, end: 23 },
    ]);
  });

  it("keeps the hidden OSC 8 destination separate from its visible label", () => {
    expect(osc8Link("https://example.test/actual", "documentation", 4, 17)).toEqual({
      kind: "url",
      text: "documentation",
      target: "https://example.test/actual",
      start: 4,
      end: 17,
    });
    expect(osc8Link("javascript:alert(1)", "unsafe", 0, 6)).toBeNull();
  });

  it("treats an OSC 8 link as naming its address only when the text is that same address", () => {
    expect(linkTextNamesTarget({ text: "https://example.test", target: "https://example.test/" })).toBe(true);
    expect(linkTextNamesTarget({ text: "https://github.com/", target: "https://evil.example/" })).toBe(false);
    expect(linkTextNamesTarget({ text: "documentation", target: "https://example.test/actual" })).toBe(false);
  });

  it("uses the platform modifier contract for direct opening", () => {
    expect(modifierOpensLink({ ctrlKey: true, metaKey: false })).toBe(true);
    expect(modifierOpensLink({ ctrlKey: false, metaKey: true })).toBe(true);
    expect(modifierOpensLink({ ctrlKey: false, metaKey: false })).toBe(false);
  });
});

describe("relative file paths", () => {
  it("keeps ~/ paths whole and passes named users and unknown extensions by", () => {
    expect(findTerminalLinks("[image] ~/shots/a.PNG (1 KB)")).toEqual([
      { kind: "relative-path", text: "~/shots/a.PNG", target: "~/shots/a.PNG", start: 8, end: 21 },
    ]);
    expect(findTerminalLinks("~ ~/notes.bak ~other/a.png ~other.png ~/a.png.bak")).toEqual([]);
    expect(findTerminalLinks("~/notes.txt ~/src/main.go").map((link) => link.target)).toEqual(["~/notes.txt", "~/src/main.go"]);
    expect(findTerminalLinks("./~other/a.png").map((link) => link.target)).toEqual(["./~other/a.png"]);
  });

  it("finds the relative path Claude Code prints for an image it sent", () => {
    expect(findTerminalLinks("› [image] artifacts/advisor-setting/2026-10-23/advisor-setting.png (16KB)")).toEqual([
      {
        kind: "relative-path",
        text: "artifacts/advisor-setting/2026-10-23/advisor-setting.png",
        target: "artifacts/advisor-setting/2026-10-23/advisor-setting.png",
        start: 10,
        end: 66,
      },
    ]);
  });

  it("takes ./ and ../ and a bare file name, and leaves the sentence's full stop", () => {
    expect(findTerminalLinks("saved ./out/a.PNG, ../b.jpeg and c.webp.").map((link) => link.target)).toEqual(["./out/a.PNG", "../b.jpeg", "c.webp"]);
  });

  it("takes the files Claude Code hands back, not only images", () => {
    expect(findTerminalLinks("Wrote report.md, scripts/plot.py:12 and out/chart.svg; packed dist/app.zip").map((link) => link.target))
      .toEqual(["report.md", "scripts/plot.py", "out/chart.svg", "dist/app.zip"]);
    expect(findTerminalLinks("recorded take.WAV and clip.mp4").map((link) => link.target)).toEqual(["take.WAV", "clip.mp4"]);
  });

  it("passes unknown extensions, words with a slash, longer names and URLs by", () => {
    expect(findTerminalLinks("and/or a.png.bak notes.draft")).toEqual([]);
    expect(findTerminalLinks("https://example.test/img/a.png").map((link) => link.kind)).toEqual(["url"]);
    expect(findTerminalLinks("/tmp/a.png").map((link) => link.kind)).toEqual(["path"]);
  });
});

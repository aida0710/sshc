import { describe, expect, it } from "vitest";
import { findTerminalLinks, linkTextNamesTarget, modifierOpensLink, osc8Link } from "./links";

describe("findTerminalLinks", () => {
  it("finds http links without swallowing sentence punctuation", () => {
    expect(findTerminalLinks("see https://example.test/a?q=1.")).toEqual([
      { kind: "url", text: "https://example.test/a?q=1", target: "https://example.test/a?q=1", start: 4, end: 30 },
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

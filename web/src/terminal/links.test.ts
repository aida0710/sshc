import { describe, expect, it } from "vitest";
import { findTerminalLinks } from "./links";

describe("findTerminalLinks", () => {
  it("finds http links without swallowing sentence punctuation", () => {
    expect(findTerminalLinks("see https://example.test/a?q=1.", false)).toEqual([
      { kind: "url", text: "https://example.test/a?q=1", target: "https://example.test/a?q=1", start: 4, end: 30 },
    ]);
  });

  it("finds remote paths and removes source locations from the target", () => {
    expect(findTerminalLinks("failed: /srv/app/main.go:42:7", true)).toEqual([
      { kind: "remote-path", text: "/srv/app/main.go:42:7", target: "/srv/app/main.go", start: 8, end: 29 },
    ]);
  });

  it("does not turn URL path segments into remote paths", () => {
    expect(findTerminalLinks("https://example.test/srv/file", true)).toHaveLength(1);
  });

  it("does not expose remote path actions for local shells", () => {
    expect(findTerminalLinks("/etc/hosts", false)).toEqual([]);
  });
});


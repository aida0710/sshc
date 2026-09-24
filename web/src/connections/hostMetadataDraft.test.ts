import { describe, expect, it } from "vitest";
import type { HostMetadata } from "../api/config";
import { formatTags, parseTags, sameHostMetadata, withOptionalChoice } from "./hostMetadataDraft";

const identity = { path: "config", alias: "bastion" };

describe("sameHostMetadata", () => {
  it("treats empty values as the missing fields the engine saves them as", () => {
    const saved: HostMetadata = { identity };
    const draft: HostMetadata = { identity, os: "", colour: "", order: 0, tags: [] };

    expect(sameHostMetadata(draft, saved)).toBe(true);
  });

  it("does not care in which order the fields were set", () => {
    const saved: HostMetadata = { identity, encoding: "shift_jis", appearance: { palette: "nord", font: "jetbrains-mono" } };
    const draft: HostMetadata = { appearance: { font: "jetbrains-mono", palette: "nord" }, encoding: "shift_jis", identity };

    expect(sameHostMetadata(draft, saved)).toBe(true);
  });

  it("sees a changed value as a change", () => {
    expect(sameHostMetadata({ identity, osc52: "deny" }, { identity })).toBe(false);
    expect(sameHostMetadata({ identity, tags: ["web", "db"] }, { identity, tags: ["db", "web"] })).toBe(false);
  });

  it("keeps a background tint of zero, which is a chosen value", () => {
    const saved: HostMetadata = { identity, appearance: { background: "sea.png" } };
    const draft: HostMetadata = { identity, appearance: { background: "sea.png", backgroundTint: 0 } };

    expect(sameHostMetadata(draft, saved)).toBe(false);
  });
});

describe("withOptionalChoice", () => {
  it("sets a chosen value and removes the field for the empty choice", () => {
    const chosen = withOptionalChoice({ identity }, "vpn", "tohoku");
    expect(chosen).toEqual({ identity, vpn: "tohoku" });
    expect(withOptionalChoice(chosen, "vpn", "")).toEqual({ identity });
  });
});

describe("tags", () => {
  it("reads comma-separated tags without blanks and writes them back", () => {
    expect(parseTags(" web, , db ,")).toEqual(["web", "db"]);
    expect(formatTags(["web", "db"])).toBe("web, db");
    expect(formatTags(undefined)).toBe("");
  });
});

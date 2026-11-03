import { describe, expect, it } from "vitest";
import { keyBarKeys, maxKeyBarKeys, maxKeyBarMoreKeys } from "../rules/keyBar.generated";
import { paletteGroups } from "./keyBarCatalog";
import { claudeCodeKeyBarLayout, defaultKeyBarLayout, insertKey, isPlaced, moveKey, moveToOtherRow, offersDirectInput, removeKey, replaceKey, type KeyBarLayout } from "./keyBarLayout";

const layout: KeyBarLayout = {
  keys: [{ key: "Escape" }, { key: "Tab" }, { key: "Control" }],
  moreKeys: [{ key: "Alt" }, { label: "|", text: "|" }],
};

describe("editing a key bar layout", () => {
  it("moves a key one place within its row and keeps it selected", () => {
    const moved = moveKey(layout, { row: "keys", index: 1 }, -1);
    expect(moved.layout.keys).toEqual([{ key: "Tab" }, { key: "Escape" }, { key: "Control" }]);
    expect(moved.slot).toEqual({ row: "keys", index: 0 });
    expect(moveKey(layout, { row: "keys", index: 2 }, 1).layout).toBe(layout);
  });

  it("moves a key to the end of the other row", () => {
    const moved = moveToOtherRow(layout, { row: "keys", index: 0 });
    expect(moved?.layout.keys).toEqual([{ key: "Tab" }, { key: "Control" }]);
    expect(moved?.layout.moreKeys.at(-1)).toEqual({ key: "Escape" });
    expect(moved?.slot).toEqual({ row: "moreKeys", index: 2 });
  });

  it("does not move a key into a full row", () => {
    const full: KeyBarLayout = { keys: Array.from({ length: maxKeyBarKeys }, (_, index) => ({ label: `${index}`, text: `${index}` })), moreKeys: [{ key: "Alt" }] };
    expect(moveToOtherRow(full, { row: "moreKeys", index: 0 })).toBeNull();
  });

  it("puts a new key after the selected one, or at the end of the row always shown", () => {
    expect(insertKey(layout, { row: "moreKeys", index: 0 }, { key: "F1" })?.layout.moreKeys).toEqual([{ key: "Alt" }, { key: "F1" }, { label: "|", text: "|" }]);
    const appended = insertKey(layout, null, { key: "Home" });
    expect(appended?.layout.keys.at(-1)).toEqual({ key: "Home" });
    expect(appended?.slot).toEqual({ row: "keys", index: 3 });
  });

  it("places a named key once, but a key the person made as often as they like", () => {
    expect(insertKey(layout, null, { key: "Alt" })).toBeNull();
    expect(insertKey(layout, null, { label: "|", text: "|" })?.layout.keys.at(-1)).toEqual({ label: "|", text: "|" });
    expect(isPlaced(layout, { key: "Tab" })).toBe(true);
    expect(isPlaced(layout, { label: "|", text: "|", enter: true })).toBe(false);
  });

  it("tells whether the layout holds the key that switches where typing goes, in either row", () => {
    expect(offersDirectInput(defaultKeyBarLayout)).toBe(true);
    expect(offersDirectInput(layout)).toBe(false);
    expect(offersDirectInput({ keys: [], moreKeys: [{ key: "DirectInput" }] })).toBe(true);
  });

  it("removes a key and replaces an edited one", () => {
    expect(removeKey(layout, { row: "keys", index: 2 }).keys).toEqual([{ key: "Escape" }, { key: "Tab" }]);
    expect(replaceKey(layout, { row: "moreKeys", index: 1 }, { label: "pipe", text: "| less" }).moreKeys[1]).toEqual({ label: "pipe", text: "| less" });
  });
});

describe("the keys on offer", () => {
  it("offers every key the engine accepts, once", () => {
    const offered = paletteGroups.flatMap((group) => group.keys.flatMap((key) => key.key === undefined ? [] : [key.key]));
    expect([...offered].sort()).toEqual([...keyBarKeys].sort());
  });

  it("starts from the layout the user chose (2026-11-02)", () => {
    expect(defaultKeyBarLayout.keys.map((key) => key.key)).toEqual(["DirectInput", "Escape", "Tab", "Control", "Ctrl+C", "ArrowLeft", "ArrowUp", "ArrowDown", "ArrowRight"]);
    expect(defaultKeyBarLayout.moreKeys.map((key) => key.key ?? key.text)).toEqual(["Alt", "Backspace", "Home", "End", "PageUp", "PageDown", "|", "-", "~", "/"]);
  });

  it("offers the @ that a Japanese keyboard types full width", () => {
    const symbols = paletteGroups.flatMap((group) => group.keys.flatMap((key) => key.text === undefined ? [] : [key.text]));
    expect(symbols).toContain("@");
  });
});

describe("the Claude Code key bar", () => {
  it("has the keys Claude Code asks for on the row always shown", () => {
    expect(claudeCodeKeyBarLayout.keys.map((key) => key.key ?? key.text)).toEqual(["Escape", "Shift+Tab", "Enter", "ArrowUp", "ArrowDown", "Ctrl+C", "/", "@", "Tab"]);
  });

  it("keeps the switch to direct input, behind 「…」", () => {
    expect(offersDirectInput(claudeCodeKeyBarLayout)).toBe(true);
    expect(claudeCodeKeyBarLayout.moreKeys).toContainEqual({ key: "DirectInput" });
  });

  it("follows the rules the engine holds a saved copy of it to", () => {
    const named = [...claudeCodeKeyBarLayout.keys, ...claudeCodeKeyBarLayout.moreKeys].flatMap((key) => key.key === undefined ? [] : [key.key]);
    expect(new Set(named).size).toBe(named.length);
    expect(claudeCodeKeyBarLayout.keys.length).toBeLessThanOrEqual(maxKeyBarKeys);
    expect(claudeCodeKeyBarLayout.moreKeys.length).toBeLessThanOrEqual(maxKeyBarMoreKeys);
  });
});

import { describe, expect, it } from "vitest";
import { keyBarKeys, maxKeyBarKeys } from "../rules/keyBar.generated";
import { paletteGroups } from "./keyBarCatalog";
import { defaultKeyBarLayout, insertKey, isPlaced, moveKey, moveToOtherRow, removeKey, replaceKey, type KeyBarLayout } from "./keyBarLayout";

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
});

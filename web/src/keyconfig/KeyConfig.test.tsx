import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KeyConfig } from "./KeyConfig";
import { defaultBindings, loadBindings, matchesShortcut, parseBindings, shortcutKey, shortcutsBlocked, storageKey } from "./bindings";

afterEach(() => { localStorage.clear(); vi.restoreAllMocks(); });

describe("application shortcuts", () => {
  it("shows and preserves the paste defaults and upgrades the former defaults safely", () => {
    render(<KeyConfig />);
    expect(screen.getByRole("button", { name: "Assign shortcut: Paste" })).toHaveTextContent("Ctrl+V / Meta+V / Ctrl+Shift+V");
    expect(parseBindings(JSON.stringify(defaultBindings))).toEqual(defaultBindings);
    expect(parseBindings(JSON.stringify({ paste: ["Ctrl+Shift+V", "Meta+V"] })).paste).toEqual(defaultBindings.paste);
    expect(parseBindings(JSON.stringify({ paste: ["Alt+V"] })).paste).toEqual(["Alt+V"]);
    expect(parseBindings(JSON.stringify({ paste: [] })).paste).toEqual([]);
    const custom = { ...defaultBindings, home: ["Ctrl+V"], paste: ["Ctrl+Shift+V", "Meta+V"] };
    expect(parseBindings(JSON.stringify(custom))).toEqual(custom);
  });

  it("records, persists, disables and restores shortcuts without accepting duplicates", () => {
    const view = render(<KeyConfig />);
    const assign = () => screen.getByRole("button", { name: "Assign shortcut: Command search" });
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "f", ctrlKey: true });
    expect(screen.getByRole("alert")).toHaveTextContent("Terminal search");
    expect(loadBindings().palette).toEqual(defaultBindings.palette);
    fireEvent.keyDown(assign(), { key: "p", ctrlKey: true, shiftKey: true });
    expect(loadBindings().palette).toEqual(["Ctrl+Shift+P"]);
    view.unmount();
    render(<KeyConfig />);
    expect(assign()).toHaveTextContent("Ctrl+Shift+P");
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    expect(loadBindings().palette).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
    expect(loadBindings()).toEqual(defaultBindings);
  });

  it("reports failed storage writes and leaves current bindings intact", () => {
    render(<KeyConfig />);
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Could not save");
    expect(loadBindings().palette).toEqual(defaultBindings.palette);
  });

  it("reflects settings changed in another tab", () => {
    render(<KeyConfig />);
    act(() => {
      localStorage.setItem(storageKey, JSON.stringify({ palette: ["Alt+K"] }));
      window.dispatchEvent(new StorageEvent("storage", { key: storageKey }));
    });
    expect(screen.getByRole("button", { name: "Assign shortcut: Command search" })).toHaveTextContent("Alt+K");
  });

  it("ignores composing/plain keys and matches exact modifiers", () => {
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "a" }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, isComposing: true }))).toBeNull();
    expect(matchesShortcut(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, altKey: true }), "palette")).toBe(false);
    expect(matchesShortcut(new KeyboardEvent("keydown", { key: "K", metaKey: true }), "palette")).toBe(true);
    expect(parseBindings('{"palette":["Ctrl+F"]}')).toEqual(defaultBindings);
    expect(parseBindings('{"palette":["a"]}')).toEqual(defaultBindings);
  });

  it("keeps shortcuts out of modal dialogs and the binding editor", () => {
    const view = render(<div role="dialog" aria-modal="true" />);
    expect(shortcutsBlocked(new KeyboardEvent("keydown"))).toBe(true);
    view.unmount();
    render(<KeyConfig />);
    const button = screen.getByRole("button", { name: "Assign shortcut: Command search" });
    let blocked = false;
    button.addEventListener("keydown", (event) => { blocked = shortcutsBlocked(event); });
    fireEvent.keyDown(button, { key: "k", ctrlKey: true });
    expect(blocked).toBe(true);
  });
});

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { fakeSettingsServer } from "../testing/settingsServer";
import { KeyConfig } from "./KeyConfig";
import { defaultBindings, matchesShortcut, shortcutKey, shortcutsBlocked } from "./bindings";
import { currentBindings, refreshPresets, type Preset } from "./presets";

let presets: Preset[];
let failWrites: boolean;
beforeEach(async () => {
  presets = [];
  failWrites = false;
  fakeSettingsServer({}, {
    read: async () => ({ schemaVersion: 10, presets: [{ id: "default" }], shortcutPresets: presets }),
    mutate: async (_path, init) => {
      if (failWrites) throw new Error("denied");
      presets = (JSON.parse(init?.body as string) as { presets: Preset[] }).presets;
      return {};
    },
  });
  await refreshPresets();
});
afterEach(() => vi.restoreAllMocks());

describe("application shortcuts", () => {
  it("shows the paste defaults and preserves saved paste bindings", () => {
    render(<KeyConfig />);
    expect(screen.getByRole("button", { name: "Assign shortcut: Paste" })).toHaveTextContent("Ctrl+V / Meta+V / Ctrl+Shift+V");
  });

  it("records, persists, disables and restores shortcuts without accepting duplicates", async () => {
    const view = render(<KeyConfig />);
    const assign = () => screen.getByRole("button", { name: "Assign shortcut: Command search" });
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "f", ctrlKey: true });
    expect(screen.getByRole("alert")).toHaveTextContent("Terminal search");
    expect(currentBindings().palette).toEqual(defaultBindings.palette);
    fireEvent.keyDown(assign(), { key: "p", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(currentBindings().palette).toEqual(["Ctrl+Shift+P"]));
    view.unmount();
    render(<KeyConfig />);
    expect(assign()).toHaveTextContent("Ctrl+Shift+P");
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    await waitFor(() => expect(currentBindings().palette).toEqual([]));
    fireEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
    await waitFor(() => expect(currentBindings()).toEqual(defaultBindings));
  });

  it("records a function key with Shift and one past F12, and saves what it recorded", async () => {
    render(<KeyConfig />);
    const assign = () => screen.getByRole("button", { name: "Assign shortcut: Open Home" });
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "F2", shiftKey: true });
    await waitFor(() => expect(currentBindings().home).toEqual(["Shift+F2"]));
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "F13" });
    await waitFor(() => expect(currentBindings().home).toEqual(["F13"]));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("does not record Shift with an ordinary key, which is typing rather than a chord", () => {
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "A", shiftKey: true }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "+", ctrlKey: true, shiftKey: true }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "F1", shiftKey: true }))).toBe("Shift+F1");
  });

  it("reports a refused save and leaves current bindings intact", async () => {
    render(<KeyConfig />);
    failWrites = true;
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    await waitFor(() => expect(screen.getAllByRole("alert").some((element) => element.textContent?.includes("Could not save"))).toBe(true));
    expect(currentBindings().palette).toEqual(defaultBindings.palette);
  });

  it("reflects shortcuts changed on another machine", async () => {
    render(<KeyConfig />);
    presets = [{ id: "work", name: "Work", bindings: { ...defaultBindings, palette: ["Alt+K"] } }];
    await act(async () => {
      await refreshPresets();
      fireEvent.change(screen.getByRole("combobox", { name: "Shortcut preset" }), { target: { value: "work" } });
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "Assign shortcut: Command search" })).toHaveTextContent("Alt+K"));
  });

  it("ignores composing/plain keys and matches exact modifiers", () => {
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "a" }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, isComposing: true }))).toBeNull();
    expect(matchesShortcut(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, altKey: true }), "palette", defaultBindings)).toBe(false);
    expect(matchesShortcut(new KeyboardEvent("keydown", { key: "K", metaKey: true }), "palette", defaultBindings)).toBe(true);
  });

  it.each([
    { isComposing: true },
    { isComposing: false, keyCode: 229 },
  ])("keeps recording through IME keys reported as %j", async (composition) => {
    render(<KeyConfig />);
    const assign = screen.getByRole("button", { name: "Assign shortcut: Open Home" });
    fireEvent.click(assign);
    for (const key of ["Escape", "Enter", "F2"]) {
      expect(fireEvent.keyDown(assign, { key, ...composition })).toBe(true);
      expect(shortcutKey(new KeyboardEvent("keydown", { key, ...composition }))).toBeNull();
      expect(shortcutsBlocked(new KeyboardEvent("keydown", { key, ...composition }))).toBe(true);
    }
    expect(apiClient.mutate).not.toHaveBeenCalled();

    fireEvent.keyDown(assign, { key: "F2" });
    await waitFor(() => expect(currentBindings().home).toEqual(["F2"]));
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

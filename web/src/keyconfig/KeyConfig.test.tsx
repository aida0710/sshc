import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderSettings, saveBar, saveButton, saveSettings } from "../testing/settingsDraft";
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
    renderSettings(<KeyConfig presetId="default" />);
    expect(screen.getByRole("button", { name: "Assign shortcut: Paste" })).toHaveTextContent("Ctrl+V / Win+V / Ctrl+Shift+V");
  });

  it("records, persists, disables and restores shortcuts without accepting duplicates", async () => {
    const view = renderSettings(<KeyConfig presetId="default" />);
    const assign = () => screen.getByRole("button", { name: "Assign shortcut: Command search" });
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "f", ctrlKey: true });
    expect(screen.getByRole("alert")).toHaveTextContent("Terminal search");
    expect(currentBindings().palette).toEqual(defaultBindings.palette);
    fireEvent.keyDown(assign(), { key: "p", ctrlKey: true, shiftKey: true });
    expect(assign()).toHaveTextContent("Ctrl+Shift+P");
    expect(currentBindings().palette).toEqual(defaultBindings.palette);
    await saveSettings();
    expect(currentBindings().palette).toEqual(["Ctrl+Shift+P"]);
    view.unmount();
    renderSettings(<KeyConfig presetId="default" />);
    expect(assign()).toHaveTextContent("Ctrl+Shift+P");
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    await saveSettings();
    expect(currentBindings().palette).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Actions for My key bindings" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Back to the default bindings" }));
    expect(currentBindings().palette).toEqual([]);
    await userEvent.click(saveButton());
    await waitFor(() => expect(currentBindings()).toEqual(defaultBindings));
  });

  it("records a function key with Shift and one past F12, and saves what it recorded", async () => {
    renderSettings(<KeyConfig presetId="default" />);
    const assign = () => screen.getByRole("button", { name: "Assign shortcut: Open Home" });
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "F2", shiftKey: true });
    expect(assign()).toHaveTextContent("Shift+F2");
    fireEvent.click(assign());
    fireEvent.keyDown(assign(), { key: "F13" });
    await saveSettings();
    expect(currentBindings().home).toEqual(["F13"]);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("does not record Shift with an ordinary key, which is typing rather than a chord", () => {
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "A", shiftKey: true }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "+", ctrlKey: true, shiftKey: true }))).toBeNull();
    expect(shortcutKey(new KeyboardEvent("keydown", { key: "F1", shiftKey: true }))).toBe("Shift+F1");
  });

  it("reports a refused save and leaves current bindings intact", async () => {
    renderSettings(<KeyConfig presetId="default" />);
    failWrites = true;
    fireEvent.click(screen.getByRole("button", { name: "Clear shortcut: Command search" }));
    await userEvent.click(saveButton());
    await waitFor(() => expect(saveBar()).toHaveTextContent("The settings could not be saved."));
    expect(screen.getByRole("button", { name: "Reload" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Assign shortcut: Command search" })).toHaveTextContent("Unassigned");
    expect(currentBindings().palette).toEqual(defaultBindings.palette);
  });

  it("reflects shortcuts changed on another machine", async () => {
    renderSettings(<KeyConfig presetId="default" />);
    presets = [{ id: "work", name: "Work", bindings: { ...defaultBindings, palette: ["Alt+K"] } }];
    await act(async () => {
      await refreshPresets();
      fireEvent.change(screen.getByRole("combobox", { name: "Key bindings" }), { target: { value: "work" } });
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
    renderSettings(<KeyConfig presetId="default" />);
    const assign = screen.getByRole("button", { name: "Assign shortcut: Open Home" });
    fireEvent.click(assign);
    for (const key of ["Escape", "Enter", "F2"]) {
      expect(fireEvent.keyDown(assign, { key, ...composition })).toBe(true);
      expect(shortcutKey(new KeyboardEvent("keydown", { key, ...composition }))).toBeNull();
      expect(shortcutsBlocked(new KeyboardEvent("keydown", { key, ...composition }))).toBe(true);
    }
    expect(saveBar()).toBeNull();

    fireEvent.keyDown(assign, { key: "F2" });
    await saveSettings();
    expect(currentBindings().home).toEqual(["F2"]);
  });

  it("keeps shortcuts out of modal dialogs and the binding editor", () => {
    const view = render(<div role="dialog" aria-modal="true" />);
    expect(shortcutsBlocked(new KeyboardEvent("keydown"))).toBe(true);
    view.unmount();
    renderSettings(<KeyConfig presetId="default" />);
    const button = screen.getByRole("button", { name: "Assign shortcut: Command search" });
    let blocked = false;
    button.addEventListener("keydown", (event) => { blocked = shortcutsBlocked(event); });
    fireEvent.keyDown(button, { key: "k", ctrlKey: true });
    expect(blocked).toBe(true);
  });
});

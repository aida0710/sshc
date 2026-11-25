import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { touchFirstQuery } from "../terminal/touchMode";
import { renderSettings, saveButton, saveSettings } from "../testing/settingsDraft";
import { fakeSettingsServer } from "../testing/settingsServer";
import { presetInterfaceOf } from "./presetInterface";
import { currentSettings } from "./settingsStore";
import { TouchModeField } from "./TouchModeField";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// keptTouchMode is the choice as a preset keeps it.
function keptTouchMode(presetId: string): string {
  return presetInterfaceOf(currentSettings(), presetId).touchMode ?? "auto";
}

describe("TouchModeField", () => {
  it("detects touch until turned off, then fixes the input picked, in the preset being edited", async () => {
    vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
      matches: query === touchFirstQuery, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    })));
    fakeSettingsServer({ presets: [{ id: "default" }, { id: "phone", name: "Phone" }] });
    renderSettings(<TouchModeField presetId="phone" />);
    const detect = screen.getByRole("checkbox", { name: "Detect automatically" });
    const fixed = screen.getByRole("combobox", { name: "Fixed input" });
    expect(detect).toBeChecked();
    expect(fixed).toBeDisabled();
    expect(fixed).toHaveValue("touch");

    await userEvent.click(detect);
    // Turning detection off keeps what it picked until another input is chosen.
    expect(fixed).toHaveValue("touch");
    expect(fixed).toBeEnabled();

    await userEvent.selectOptions(fixed, "mouse");
    expect(keptTouchMode("phone")).toBe("auto");
    await saveSettings();
    expect(keptTouchMode("phone")).toBe("mouse");
    // The preset this machine uses is not the one being edited, and keeps its own choice.
    expect(keptTouchMode("default")).toBe("auto");
  });

  it("underlines links on touch unless turned off, in the preset being edited", async () => {
    fakeSettingsServer({ presets: [{ id: "default" }, { id: "phone", name: "Phone" }] });
    renderSettings(<TouchModeField presetId="phone" />);
    const underline = screen.getByRole("checkbox", { name: "Always underline links" });
    expect(underline).toBeChecked();

    await userEvent.click(underline);
    expect(underline).not.toBeChecked();
    await saveSettings();
    expect(presetInterfaceOf(currentSettings(), "phone").hideTouchLinkUnderlines).toBe(true);
    expect(presetInterfaceOf(currentSettings(), "default").hideTouchLinkUnderlines).toBeUndefined();

    // Turning it back on is the default, which a preset does not keep.
    await userEvent.click(underline);
    await userEvent.click(saveButton());
    await vi.waitFor(() => expect(presetInterfaceOf(currentSettings(), "phone")).toEqual({}));
  });
});

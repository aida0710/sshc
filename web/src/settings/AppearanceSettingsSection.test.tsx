import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderSettings, saveButton, saveSettings } from "../testing/settingsDraft";
import { fakeSettingsServer } from "../testing/settingsServer";
import { AccentProvider } from "../theme/accentContext";
import { ThemeProvider } from "../theme/context";
import { MotionProvider } from "../theme/motionContext";
import { AppearanceSettingsSection } from "./AppearanceSettingsSection";

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
});

afterEach(() => {
  vi.restoreAllMocks();
  document.documentElement.removeAttribute("data-accent");
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-motion");
  document.documentElement.removeAttribute("style");
});

function renderSection() {
  return renderSettings(
    <ThemeProvider>
      <AccentProvider>
        <MotionProvider>
          <AppearanceSettingsSection presetId="default" showHeading />
        </MotionProvider>
      </AccentProvider>
    </ThemeProvider>,
  );
}


// accentsWritten is every accent saved in the standard preset, oldest first.
function accentsWritten(server: ReturnType<typeof fakeSettingsServer>): unknown[] {
  return server.writesTo("/api/v1/settings/presets/default")
    .map((body) => (body as { preset: { appearance?: { accent?: string } } }).preset.appearance?.accent);
}

describe("AppearanceSettingsSection", () => {
  it("offers the five presets and a custom colour with lavender chosen by default", () => {
    fakeSettingsServer();
    renderSection();

    const accents = screen.getByRole("radiogroup", { name: "Accent colour" });
    const names = within(accents).getAllByRole("radio").map((radio) => radio.closest("label")?.textContent);
    expect(names).toEqual(["Lavender", "Blue", "Sky", "Pink", "Mono", "Custom"]);
    expect(within(accents).getByRole("radio", { name: "Lavender" })).toBeChecked();
  });

  it("paints the page with the accent once it is saved, not when it is picked", async () => {
    const server = fakeSettingsServer();
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Sky" }));
    expect(screen.getByRole("radio", { name: "Sky" })).toBeChecked();
    expect(document.documentElement).toHaveAttribute("data-accent", "lavender");

    await saveSettings();
    expect(accentsWritten(server)).toEqual(["sky"]);
    expect(document.documentElement).toHaveAttribute("data-accent", "sky");
  });

  it("keeps the previous accent on the page when the change could not be saved", async () => {
    fakeSettingsServer();
    const { apiClient } = await import("../api/client");
    vi.mocked(apiClient.mutate).mockRejectedValue(new Error("disk full"));
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Pink" }));
    await userEvent.click(saveButton());

    expect(await screen.findByText("The settings could not be saved.")).toBeVisible();
    expect(screen.getByRole("radio", { name: "Pink" })).toBeChecked();
    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "lavender"));
  });

  it("starts a custom accent from the colour on screen and saves it", async () => {
    const server = fakeSettingsServer();
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));

    // The test page has no stylesheet, so the colour on screen is the lavender fallback.
    expect(screen.getByRole("textbox", { name: "Custom colour code" })).toHaveValue("#6550d0");
    await saveSettings();
    expect(accentsWritten(server)).toEqual(["#6550d0"]);
    expect(document.documentElement).toHaveAttribute("data-accent", "custom");
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent")).toBe("#6550d0");
  });

  it("saves a typed colour code and says when it cannot read one", async () => {
    const server = fakeSettingsServer();
    renderSection();
    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));
    const code = await screen.findByRole("textbox", { name: "Custom colour code" });

    await userEvent.clear(code);
    await userEvent.type(code, "green");
    expect(screen.getByText("Enter the colour as #rrggbb.")).toBeVisible();
    expect(saveButton()).toBeDisabled();

    await userEvent.clear(code);
    await userEvent.type(code, "3FA86B");
    await saveSettings();
    expect(accentsWritten(server)).toEqual(["#3fa86b"]);
  });

  it("gives up the custom colour when a preset is chosen", async () => {
    const server = fakeSettingsServer();
    renderSection();
    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));
    await screen.findByRole("textbox", { name: "Custom colour code" });

    await userEvent.click(screen.getByRole("radio", { name: "Blue" }));
    expect(screen.queryByRole("textbox", { name: "Custom colour code" })).toBeNull();

    await saveSettings();
    expect(accentsWritten(server)).toEqual(["blue"]);
    expect(document.documentElement).toHaveAttribute("data-accent", "blue");
  });

  it("switches the theme once it is saved", async () => {
    fakeSettingsServer({ presets: [{ id: "default", appearance: { theme: "dark" } }] });
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Light" }));
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");

    await saveSettings();
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });
});

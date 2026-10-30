import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { AccentProvider } from "../theme/accentContext";
import { ThemeProvider } from "../theme/context";
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
  document.documentElement.removeAttribute("style");
});

function renderSection() {
  return render(
    <ThemeProvider initial="dark">
      <AccentProvider>
        <AppearanceSettingsSection presetId="default" showHeading />
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

  it("saves the accent the user picks in the preset in use and paints the page with it", async () => {
    const server = fakeSettingsServer();
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Sky" }));

    expect(document.documentElement).toHaveAttribute("data-accent", "sky");
    await waitFor(() => expect(accentsWritten(server)).toEqual(["sky"]));
    await waitFor(() => expect(screen.getByRole("radio", { name: "Sky" })).toBeChecked());
  });

  it("says the accent was not saved and keeps the previous one", async () => {
    fakeSettingsServer();
    const { apiClient } = await import("../api/client");
    vi.mocked(apiClient.mutate).mockRejectedValue(new Error("disk full"));
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Pink" }));

    expect(await screen.findByText("The accent colour could not be saved. Choose it again.")).toBeVisible();
    expect(screen.getByRole("radio", { name: "Lavender" })).toBeChecked();
    expect(document.documentElement).toHaveAttribute("data-accent", "lavender");
  });

  it("starts a custom accent from the colour on screen and saves it", async () => {
    const server = fakeSettingsServer();
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));

    // The test page has no stylesheet, so the colour on screen is the lavender fallback.
    await waitFor(() => expect(accentsWritten(server)).toEqual(["#6550d0"]));
    expect(document.documentElement).toHaveAttribute("data-accent", "custom");
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent")).toBe("#6550d0");
    expect(await screen.findByRole("textbox", { name: "Custom colour code" })).toHaveValue("#6550d0");
  });

  it("saves a typed colour code and says when it cannot read one", async () => {
    const server = fakeSettingsServer();
    renderSection();
    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));
    const code = await screen.findByRole("textbox", { name: "Custom colour code" });

    await userEvent.clear(code);
    await userEvent.type(code, "3FA86B{Enter}");
    await waitFor(() => expect(accentsWritten(server).at(-1)).toBe("#3fa86b"));

    await userEvent.clear(code);
    await userEvent.type(code, "green{Enter}");
    expect(screen.getByText("Enter the colour as #rrggbb.")).toBeVisible();
    expect(accentsWritten(server)).toHaveLength(2);
  });

  it("gives up the custom colour when a preset is chosen", async () => {
    const server = fakeSettingsServer();
    renderSection();
    await userEvent.click(screen.getByRole("radio", { name: "Custom" }));
    await screen.findByRole("textbox", { name: "Custom colour code" });

    await userEvent.click(screen.getByRole("radio", { name: "Blue" }));

    await waitFor(() => expect(accentsWritten(server).at(-1)).toBe("blue"));
    expect(document.documentElement).toHaveAttribute("data-accent", "blue");
    expect(screen.queryByRole("textbox", { name: "Custom colour code" })).toBeNull();
  });

  it("switches the theme from the same page", async () => {
    fakeSettingsServer();
    renderSection();

    await userEvent.click(screen.getByRole("radio", { name: "Light" }));

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });
});

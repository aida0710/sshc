import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppearanceApi } from "../api/appearance";
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
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-accent");
  document.documentElement.removeAttribute("data-theme");
});

function renderSection(api: AppearanceApi) {
  return render(
    <ThemeProvider initial="dark">
      <AccentProvider api={api}>
        <AppearanceSettingsSection showHeading />
      </AccentProvider>
    </ThemeProvider>,
  );
}

function fakeApi(): AppearanceApi & { setAppearanceSettings: ReturnType<typeof vi.fn> } {
  return {
    appearanceSettings: vi.fn(async () => ({})),
    setAppearanceSettings: vi.fn(async () => undefined),
  };
}

describe("AppearanceSettingsSection", () => {
  it("offers the five accents with lavender chosen by default", () => {
    renderSection(fakeApi());

    const accents = screen.getByRole("radiogroup", { name: "Accent colour" });
    const names = within(accents).getAllByRole("radio").map((radio) => radio.closest("label")?.textContent);
    expect(names).toEqual(["Lavender", "Blue", "Sky", "Pink", "Mono"]);
    expect(within(accents).getByRole("radio", { name: "Lavender" })).toBeChecked();
  });

  it("saves the accent the user picks and paints the page with it", async () => {
    const api = fakeApi();
    renderSection(api);

    await userEvent.click(screen.getByRole("radio", { name: "Sky" }));

    expect(api.setAppearanceSettings).toHaveBeenCalledWith({ accent: "sky" });
    expect(document.documentElement).toHaveAttribute("data-accent", "sky");
    await waitFor(() => expect(screen.getByRole("radio", { name: "Sky" })).toBeChecked());
  });

  it("says the accent was not saved and keeps the previous one", async () => {
    const api = fakeApi();
    api.setAppearanceSettings.mockRejectedValue(new Error("disk full"));
    renderSection(api);

    await userEvent.click(screen.getByRole("radio", { name: "Pink" }));

    expect(await screen.findByText("The accent colour could not be saved. Choose it again.")).toBeVisible();
    expect(screen.getByRole("radio", { name: "Lavender" })).toBeChecked();
    expect(document.documentElement).toHaveAttribute("data-accent", "lavender");
  });

  it("switches the theme from the same page", async () => {
    renderSection(fakeApi());

    await userEvent.click(screen.getByRole("radio", { name: "Light" }));

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });
});

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { fakeSettingsServer } from "../testing/settingsServer";
import { AccentProvider, useAccent } from "./accentContext";

afterEach(() => {
  vi.restoreAllMocks();
  document.documentElement.removeAttribute("data-accent");
  document.documentElement.removeAttribute("style");
});

function Probe() {
  const { accent, chooseAccent } = useAccent();
  return (
    <div>
      <span>{`accent ${accent}`}</span>
      <button type="button" onClick={() => void chooseAccent("pink").catch(() => undefined)}>choose pink</button>
    </div>
  );
}

describe("AccentProvider", () => {
  it("shows the default lavender before the settings arrive", () => {
    render(<AccentProvider><Probe /></AccentProvider>);

    expect(screen.getByText("accent lavender")).toBeVisible();
    expect(document.documentElement).toHaveAttribute("data-accent", "lavender");
  });

  it("puts the accent of the preset this machine uses on the page", async () => {
    fakeSettingsServer({ presets: [{ id: "default", appearance: { accent: "sky" } }] });
    render(<AccentProvider><Probe /></AccentProvider>);

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "sky"));
  });

  it("puts a saved colour of one's own on the page", async () => {
    fakeSettingsServer({ presets: [{ id: "default", appearance: { accent: "#3fa86b" } }] });
    render(<AccentProvider><Probe /></AccentProvider>);

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "custom"));
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent")).toBe("#3fa86b");
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent-ink")).toBe("#111214");
  });

  it("follows the preset this machine uses, not the standard one", async () => {
    fakeSettingsServer({
      presets: [{ id: "default", appearance: { accent: "sky" } }, { id: "phone", name: "Phone", appearance: { accent: "pink" } }],
      presetInUse: "phone",
    });
    render(<AccentProvider><Probe /></AccentProvider>);

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "pink"));
  });

  it("shows a chosen accent at once and saves it in the preset in use", async () => {
    const server = fakeSettingsServer();
    render(<AccentProvider><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "choose pink" }));

    expect(document.documentElement).toHaveAttribute("data-accent", "pink");
    await waitFor(() => expect(server.writesTo("/api/v1/settings/presets/default")).toEqual([
      { base: { id: "default" }, preset: { id: "default", appearance: { accent: "pink" } } },
    ]));
  });

  it("puts the saved accent back when saving fails", async () => {
    fakeSettingsServer();
    render(<AccentProvider><Probe /></AccentProvider>);
    const { apiClient } = await import("../api/client");
    vi.mocked(apiClient.mutate).mockRejectedValue(new ApiError("disk_full", 500, null));

    await userEvent.click(screen.getByRole("button", { name: "choose pink" }));

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "lavender"));
  });
});

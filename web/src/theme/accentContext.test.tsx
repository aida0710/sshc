import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AppearanceApi, AppearanceSettings } from "../api/appearance";
import { AccentProvider, useAccent } from "./accentContext";

afterEach(() => {
  document.documentElement.removeAttribute("data-accent");
  document.documentElement.removeAttribute("style");
});

function fakeApi(saved: AppearanceSettings = {}): AppearanceApi & { setAppearanceSettings: ReturnType<typeof vi.fn> } {
  return {
    appearanceSettings: vi.fn(async () => saved),
    setAppearanceSettings: vi.fn(async () => undefined),
  };
}

function Probe() {
  const { accent, loadAccent, chooseAccent } = useAccent();
  return (
    <div>
      <span>{`accent ${accent}`}</span>
      <button type="button" onClick={loadAccent}>load</button>
      <button type="button" onClick={() => void chooseAccent("pink").catch(() => undefined)}>choose pink</button>
    </div>
  );
}

describe("AccentProvider", () => {
  it("shows the default lavender until the saved accent is read", () => {
    render(<AccentProvider api={fakeApi({ accent: "sky" })}><Probe /></AccentProvider>);

    expect(screen.getByText("accent lavender")).toBeVisible();
    expect(document.documentElement).toHaveAttribute("data-accent", "lavender");
  });

  it("puts the saved accent on the page when asked to load it", async () => {
    render(<AccentProvider api={fakeApi({ accent: "sky" })}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "load" }));

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "sky"));
  });

  it("puts a saved colour of one's own on the page over its preset", async () => {
    render(<AccentProvider api={fakeApi({ accent: "sky", accentColor: "#3fa86b" })}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "load" }));

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "custom"));
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent")).toBe("#3fa86b");
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent-ink")).toBe("#111214");
  });

  it("shows the preset when the saved colour cannot be read", async () => {
    render(<AccentProvider api={fakeApi({ accent: "pink", accentColor: "green" })}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "load" }));

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "pink"));
    expect(document.documentElement.style.getPropertyValue("--ui-custom-accent")).toBe("");
  });

  it("falls back to lavender when nothing was saved", async () => {
    render(<AccentProvider api={fakeApi({})}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "load" }));

    await waitFor(() => expect(screen.getByText("accent lavender")).toBeVisible());
  });

  it("shows a chosen accent at once and saves it", async () => {
    const api = fakeApi();
    render(<AccentProvider api={api}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "choose pink" }));

    expect(document.documentElement).toHaveAttribute("data-accent", "pink");
    expect(api.setAppearanceSettings).toHaveBeenCalledWith({ accent: "pink" });
  });

  it("puts the previous accent back when saving fails", async () => {
    const api = fakeApi();
    api.setAppearanceSettings.mockRejectedValue(new Error("disk full"));
    render(<AccentProvider api={api}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "choose pink" }));

    await waitFor(() => expect(document.documentElement).toHaveAttribute("data-accent", "lavender"));
  });

  it("keeps the accent the user chose over a read that arrives later", async () => {
    let answer: (settings: AppearanceSettings) => void = () => undefined;
    const api = fakeApi();
    api.appearanceSettings = vi.fn(() => new Promise<AppearanceSettings>((resolve) => { answer = resolve; }));
    render(<AccentProvider api={api}><Probe /></AccentProvider>);

    await userEvent.click(screen.getByRole("button", { name: "load" }));
    await userEvent.click(screen.getByRole("button", { name: "choose pink" }));
    await act(async () => answer({ accent: "blue" }));

    expect(document.documentElement).toHaveAttribute("data-accent", "pink");
  });
});

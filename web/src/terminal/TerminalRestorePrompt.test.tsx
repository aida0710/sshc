import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalRestoreApi } from "../api/terminalRestore";
import { LanguageProvider } from "../i18n/context";
import { TerminalRestorePrompt } from "./TerminalRestorePrompt";

function restoreApi(overrides: Partial<TerminalRestoreApi> = {}): TerminalRestoreApi {
  return {
    offer: vi.fn().mockResolvedValue({
      sessions: [
        { kind: "ssh", alias: "edge", workspaceId: "default", workingDirectory: "~/src" },
        { kind: "shell", workspaceId: "default", pinnedTitle: "build" },
        { kind: "shell", workspaceId: "default" },
      ],
    }),
    restore: vi.fn().mockResolvedValue({ opened: 3, failed: 0 }),
    decline: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

function renderPrompt(api: TerminalRestoreApi, onRestored = vi.fn().mockResolvedValue(undefined)) {
  render(
    <LanguageProvider initial="en">
      <TerminalRestorePrompt api={api} onRestored={onRestored} />
    </LanguageProvider>,
  );
  return onRestored;
}

describe("TerminalRestorePrompt", () => {
  it("names the offered sessions and where they worked", async () => {
    renderPrompt(restoreApi());
    expect(await screen.findByRole("dialog", { name: "Reopen your last sessions?" })).toBeTruthy();
    expect(screen.getByText("edge")).toBeTruthy();
    expect(screen.getByText("~/src")).toBeTruthy();
    expect(screen.getByText("build")).toBeTruthy();
    expect(screen.getByText("Local shell")).toBeTruthy();
  });

  it("reopens the sessions and refreshes the session list", async () => {
    const api = restoreApi();
    const onRestored = renderPrompt(api);
    fireEvent.click(await screen.findByRole("button", { name: "Reopen" }));
    await waitFor(() => expect(onRestored).toHaveBeenCalled());
    expect(api.restore).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("declining ends the offer without reopening anything", async () => {
    const api = restoreApi();
    const onRestored = renderPrompt(api);
    fireEvent.click(await screen.findByRole("button", { name: "Don't reopen" }));
    await waitFor(() => expect(api.decline).toHaveBeenCalledTimes(1));
    expect(api.restore).not.toHaveBeenCalled();
    expect(onRestored).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks nothing when there is no offer or it cannot be read", async () => {
    const offer = vi.fn().mockRejectedValue(new Error("engine unavailable"));
    renderPrompt(restoreApi({ offer }));
    await waitFor(() => expect(offer).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the question open with the reason when reopening fails", async () => {
    renderPrompt(restoreApi({ restore: vi.fn().mockRejectedValue(new Error("refused")) }));
    fireEvent.click(await screen.findByRole("button", { name: "Reopen" }));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", "The sessions could not be reopened.");
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});

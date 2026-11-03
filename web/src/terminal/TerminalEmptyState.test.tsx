import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TerminalEmptyState } from "./TerminalEmptyState";

function renderEmptyState(runTarget: string | null) {
  const handlers = { onOpenLocalShell: vi.fn(), onAddConnection: vi.fn(), onOpenSync: vi.fn() };
  render(<TerminalEmptyState runTarget={runTarget} {...handlers} />);
  return handlers;
}

describe("TerminalEmptyState", () => {
  it("opens a shell, adds a connection or goes to Sync from its tiles", async () => {
    const handlers = renderEmptyState(null);

    await userEvent.click(screen.getByRole("button", { name: /^Open a local shell/ }));
    expect(handlers.onOpenLocalShell).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: /^Add connection/ }));
    expect(handlers.onAddConnection).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: /^Receive by sync/ }));
    expect(handlers.onOpenSync).toHaveBeenCalledOnce();
  });

  it("says the shell opens on this machine when no run target is chosen", () => {
    renderEmptyState(null);
    expect(screen.getByText("Open a shell on this machine right away")).toBeInTheDocument();
  });

  it("names the run target a new shell would open on", () => {
    renderEmptyState("office-pc");
    expect(screen.getByText("Open a shell on office-pc right away")).toBeInTheDocument();
  });
});

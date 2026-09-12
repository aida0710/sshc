import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ModalShell } from "./ModalShell";

describe("ModalShell", () => {
  it("portals a modal, traps focus, and dismisses it with Escape", async () => {
    const dismiss = vi.fn();
    render(
      <div data-testid="clipper">
        <ModalShell labelledBy="modal-heading" onDismiss={dismiss}>
          <h2 id="modal-heading">Shared modal</h2>
          <button type="button">First</button>
          <button type="button">Last</button>
        </ModalShell>
      </div>,
    );

    const dialog = screen.getByRole("dialog", { name: "Shared modal" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByTestId("clipper").contains(dialog)).toBe(false);
    expect(screen.getByRole("button", { name: "First" })).toHaveFocus();
    await userEvent.tab({ shift: true });
    expect(screen.getByRole("button", { name: "Last" })).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    expect(dismiss).toHaveBeenCalledWith("escape");
  });
});

it("skips collapsed and hidden controls when choosing and trapping focus", async () => {
  render(<ModalShell labelledBy="hidden-controls-heading" onDismiss={vi.fn()}>
    <h2 id="hidden-controls-heading">Hidden controls</h2>
    <div hidden><button>Hidden first</button></div>
    <button>Visible first</button>
    <button>Visible last</button>
    <div style={{ display: "none" }}><button>Hidden last</button></div>
  </ModalShell>);
  expect(screen.getByRole("button", { name: "Visible first" })).toHaveFocus();
  await userEvent.tab({ shift: true });
  expect(screen.getByRole("button", { name: "Visible last" })).toHaveFocus();
  await userEvent.tab();
  expect(screen.getByRole("button", { name: "Visible first" })).toHaveFocus();
});

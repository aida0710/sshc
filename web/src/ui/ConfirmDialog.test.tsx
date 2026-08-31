import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

function open(overrides: Partial<Parameters<typeof ConfirmDialog>[0]> = {}) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const view = render(
    <div data-testid="clipper" className="overflow-hidden" style={{ transform: "translateX(0)", width: "288px" }}>
      <ConfirmDialog
        id="heading"
        heading="Close zsh?"
        body={<p>Everything running stops.</p>}
        confirmLabel="Close"
        cancelLabel="Keep it open"
        onConfirm={onConfirm}
        onCancel={onCancel}
        {...overrides}
      />
    </div>,
  );
  return { view, onConfirm, onCancel };
}

describe("ConfirmDialog", () => {
  it("hangs outside whatever rendered it", () => {
    open();
    const dialog = screen.getByRole("dialog");
    expect(screen.getByTestId("clipper").contains(dialog)).toBe(false);
    expect(document.body.contains(dialog)).toBe(true);
  });

  it("starts on the side that loses nothing", () => {
    open();
    expect(screen.getByRole("button", { name: "Keep it open" })).toHaveFocus();
  });

  it("takes Escape as leaving it alone", async () => {
    const { onCancel, onConfirm } = open();
    await userEvent.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("keeps keyboard focus inside the modal", async () => {
    open();
    const user = userEvent.setup();
    const cancel = screen.getByRole("button", { name: "Keep it open" });
    const confirm = screen.getByRole("button", { name: "Close" });

    expect(cancel).toHaveFocus();
    await user.tab({ shift: true });
    expect(confirm).toHaveFocus();
    await user.tab();
    expect(cancel).toHaveFocus();
  });
});

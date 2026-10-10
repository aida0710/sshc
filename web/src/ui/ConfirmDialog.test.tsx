import { render, screen, waitFor } from "@testing-library/react";
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

  it("keeps late terminal focus from leaving the confirmation", () => {
    const input = document.createElement("textarea");
    document.body.append(input);
    open();
    input.focus();
    expect(screen.getByRole("button", { name: "Keep it open" })).toHaveFocus();
    input.remove();
  });

  it("takes Escape as leaving it alone", async () => {
    const { onCancel, onConfirm } = open();
    await userEvent.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("disables both buttons and ignores Escape until the confirmed action settles", async () => {
    let settle: () => void = () => undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => { settle = resolve; }));
    const { onCancel } = open({ onConfirm });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Keep it open" })).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(onCancel).not.toHaveBeenCalled();
    expect(onConfirm).toHaveBeenCalledTimes(1);

    settle();
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toBeEnabled());
  });

  it("keeps focus inside the dialog when Escape is pressed while the confirmed action runs", async () => {
    const opener = document.createElement("button");
    opener.textContent = "Open";
    document.body.append(opener);
    opener.focus();
    let settle: () => void = () => undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => { settle = resolve; }));
    open({ onConfirm });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Close" }));
    await user.keyboard("{Escape}");
    await Promise.resolve();

    expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement);
    settle();
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toBeEnabled());
    opener.remove();
  });

  it("puts focus back on the confirm button when the action fails and the dialog stays open", async () => {
    let fail: () => void = () => undefined;
    const onConfirm = vi.fn(() => new Promise<void>((_resolve, reject) => { fail = () => reject(new Error("refused")); }));
    open({ onConfirm });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Close" }));
    // ブラウザは無効になったボタンからフォーカスを外す。jsdom は外さないので、処理中に
    // ダイアログの外へフォーカスが移った状態を作る。両方のボタンが無効なので、ダイアログは
    // フォーカスを引き戻せない。
    const outside = document.createElement("input");
    document.body.append(outside);
    outside.focus();
    expect(outside).toHaveFocus();
    fail();

    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toHaveFocus());
    outside.remove();
  });

  it("disables both buttons while the caller reports busy", () => {
    open({ busy: true });
    expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Keep it open" })).toBeDisabled();
  });

  it("holds only the confirm button while the caller has not got what is being confirmed", async () => {
    const { onCancel, onConfirm } = open({ confirmDisabled: true });
    const user = userEvent.setup();
    const cancel = screen.getByRole("button", { name: "Keep it open" });

    expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
    expect(cancel).toBeEnabled();
    expect(cancel).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("describes the dialog with its body so the consequence is read with the focused button", () => {
    open();
    expect(screen.getByRole("dialog")).toHaveAccessibleDescription("Everything running stops.");
  });

  it("shows a failure inside the dialog and adds it to the dialog's description", () => {
    open({ error: "The file could not be deleted." });
    const dialog = screen.getByRole("dialog");
    const alert = screen.getByRole("alert");
    expect(dialog).toContainElement(alert);
    expect(alert).toHaveTextContent("The file could not be deleted.");
    expect(dialog).toHaveAccessibleDescription("Everything running stops. The file could not be deleted.");
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

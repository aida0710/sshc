import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

function open(overrides: Partial<Parameters<typeof ConfirmDialog>[0]> = {}) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const view = render(
    // 出した側を、切る側の中に置く。**ナビゲーションの板がこの形である。**
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
  // **これがこの部品の置き場所そのものである。**
  //
  // `fixed` が窓を基準にするのは、祖先が transform を持っていないときだけで
  // ある。ナビゲーションの板は開閉のために常に translate を持ち、さらに
  // overflow-hidden で切る——中に置けば、幅 288px の板に閉じ込められて
  // 文も釦も見切れる。実際にそうなっていた。
  it("hangs outside whatever rendered it", () => {
    open();
    const dialog = screen.getByRole("dialog");
    expect(screen.getByTestId("clipper").contains(dialog)).toBe(false);
    expect(document.body.contains(dialog)).toBe(true);
  });

  // 何も読まずに Enter を叩いた人が落ちる先は、失うものが無い方である。
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
});

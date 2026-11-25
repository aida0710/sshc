import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DraftSaveBar } from "./DraftSaveBar";
import { saveBarSlotClass, SaveBarSlotContext, useSaveBarSlot } from "./saveBarSlot";

describe("DraftSaveBar", () => {
  it("offers to save or discard, and says there are unsaved changes", async () => {
    const onSave = vi.fn();
    const onDiscard = vi.fn();
    render(<DraftSaveBar onSave={onSave} onDiscard={onDiscard} />);

    expect(screen.getByText("Unsaved changes")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Discard changes" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onDiscard).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("says why it cannot save yet, and why the last save failed", () => {
    const { rerender } = render(<DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} saveDisabled note="Open the vault first." />);
    expect(screen.getByText("Open the vault first.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    rerender(<DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} note="Open the vault first." failure="The settings could not be saved." />);
    expect(screen.getByText("The settings could not be saved.")).toBeVisible();
    expect(screen.queryByText("Open the vault first.")).not.toBeInTheDocument();
  });

  it("asks to fix what was typed, unless there is a reason of its own to give", () => {
    const { rerender } = render(<DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} inputInvalid />);
    expect(screen.getByText("Correct the fields to save")).toBeVisible();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    rerender(<DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} inputInvalid note="Open the vault first." />);
    expect(screen.getByText("Open the vault first.")).toBeVisible();
  });

  it("brings the error of the field typed in into view", () => {
    const shown: Element[] = [];
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = function (this: Element) { shown.push(this); };
    const page = (inputInvalid: boolean) => (
      <>
        <input aria-label="Port" aria-describedby={inputInvalid ? "hint error" : "hint"} />
        <p id="hint">Blank uses 22.</p>
        {inputInvalid ? <p id="error">Port must be 1 to 65535.</p> : null}
        <DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} inputInvalid={inputInvalid} />
      </>
    );
    const { rerender } = render(page(false));
    screen.getByLabelText("Port").focus();
    rerender(page(true));
    expect(shown).toEqual([screen.getByText("Port must be 1 to 65535.")]);
    Element.prototype.scrollIntoView = original;
  });

  it("holds both buttons while it saves", () => {
    render(<DraftSaveBar onSave={vi.fn()} onDiscard={vi.fn()} saving />);
    expect(screen.getByRole("button", { name: "Saving…" })).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Discard changes" })).toBeDisabled();
  });

  function Pane({ onSubmit }: { onSubmit: () => void }) {
    const { slot, slotRef } = useSaveBarSlot();
    return (
      <SaveBarSlotContext.Provider value={slot}>
        <form id="form" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
          <input aria-label="Port" />
          <DraftSaveBar form="form" onDiscard={vi.fn()} />
        </form>
        <div ref={slotRef} data-testid="slot" className={saveBarSlotClass} />
      </SaveBarSlotContext.Provider>
    );
  }

  it("is placed at the end of its page, and still submits the form it came from, by its button or Enter", async () => {
    const onSubmit = vi.fn();
    render(<Pane onSubmit={onSubmit} />);
    await userEvent.click(within(screen.getByTestId("slot")).getByRole("button", { name: "Save" }));
    await userEvent.type(screen.getByLabelText("Port"), "22{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });
});

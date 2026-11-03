import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ModalShell } from "./ModalShell";

// screenLaidOutAsPhone answers every media query the way a phone answers mobileViewportQuery.
function screenLaidOutAsPhone() {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: true, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

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

function DialogWithField({ asInitialFocus }: { asInitialFocus: boolean }) {
  const field = useRef<HTMLInputElement>(null);
  return <ModalShell labelledBy="field-heading" onDismiss={vi.fn()} {...(asInitialFocus ? { initialFocusRef: field } : {})}>
    <h2 id="field-heading">Field</h2>
    <input ref={field} aria-label="Value" />
    <button>Save</button>
  </ModalShell>;
}

it.each([
  ["it is the first thing in the dialog", false],
  ["the dialog asks for it", true],
])("focuses a text field on a PC when %s", (_reason, asInitialFocus) => {
  render(<DialogWithField asInitialFocus={asInitialFocus} />);
  expect(screen.getByRole("textbox", { name: "Value" })).toHaveFocus();
});

it.each([
  ["it is the first thing in the dialog", false],
  ["the dialog asks for it", true],
])("focuses the panel on a phone when %s, so the keyboard stays down", (_reason, asInitialFocus) => {
  screenLaidOutAsPhone();
  render(<DialogWithField asInitialFocus={asInitialFocus} />);
  expect(screen.getByRole("dialog", { name: "Field" })).toHaveFocus();
});

it("takes the focus back from outside to the panel on a phone, not to its text field", () => {
  screenLaidOutAsPhone();
  render(<><button>Outside</button><DialogWithField asInitialFocus /></>);
  screen.getByRole("button", { name: "Outside", hidden: true }).focus();
  expect(screen.getByRole("dialog", { name: "Field" })).toHaveFocus();
});

// F2 stands for what opens a dialog from a text field, such as sending several lines from the phone's text box.
function DialogOpenedFromField() {
  const [open, setOpen] = useState(false);
  return <>
    <input aria-label="Composer" onKeyDown={(event) => { if (event.key === "F2") setOpen(true); }} />
    {open ? <ModalShell labelledBy="closing-heading" onDismiss={() => setOpen(false)}>
      <h2 id="closing-heading">Closing</h2>
      <button onClick={() => setOpen(false)}>Close</button>
    </ModalShell> : null}
  </>;
}

async function openAndCloseDialogFromField() {
  screen.getByRole("textbox", { name: "Composer" }).focus();
  await userEvent.keyboard("{F2}");
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
}

it("gives the focus back to the text field it was opened from on a PC", async () => {
  render(<DialogOpenedFromField />);
  await openAndCloseDialogFromField();
  await vi.waitFor(() => expect(screen.getByRole("textbox", { name: "Composer" })).toHaveFocus());
});

it("leaves the keyboard down when a dialog opened from a text field closes on a phone", async () => {
  screenLaidOutAsPhone();
  render(<DialogOpenedFromField />);
  await openAndCloseDialogFromField();
  // The focus goes back in a microtask, which has run by the time this await returns.
  await Promise.resolve();
  expect(screen.getByRole("textbox", { name: "Composer" })).not.toHaveFocus();
});

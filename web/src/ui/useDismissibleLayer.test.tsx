import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { keyboardOwnerProps, useDismissibleLayer } from "./useDismissibleLayer";

function Layer({ name, close }: { name: string; close: () => void }) {
  const panel = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: true, containerRefs: [panel], onDismiss: close });
  return <div ref={panel}>{name}</div>;
}

describe("useDismissibleLayer", () => {
  it("replaces the previous layer and dismisses only the topmost layer afterward", () => {
    const lower = vi.fn();
    const upper = vi.fn();
    render(<><Layer name="lower" close={lower} /><Layer name="upper" close={upper} /></>);

    expect(lower).toHaveBeenCalledOnce();
    lower.mockClear();
    fireEvent.pointerDown(document.body);

    expect(upper).toHaveBeenCalledOnce();
    expect(lower).not.toHaveBeenCalled();
  });

  it("preserves the outside click target but restores the trigger for Escape", async () => {
    function Fixture() {
      const [open, setOpen] = useState(false);
      const trigger = useRef<HTMLButtonElement>(null);
      const panel = useRef<HTMLDivElement>(null);
      useDismissibleLayer({
        open,
        containerRefs: [panel, trigger],
        onDismiss: () => setOpen(false),
        returnFocusRef: trigger,
      });
      return <><button ref={trigger} onClick={() => setOpen((value) => !value)}>Trigger</button>{open ? <div ref={panel}>Panel <button onClick={() => setOpen(false)}>Close</button></div> : null}<button>Outside</button></>;
    }
    const user = userEvent.setup();
    render(<Fixture />);
    const trigger = screen.getByRole("button", { name: "Trigger" });
    const outside = screen.getByRole("button", { name: "Outside" });

    await user.click(trigger);
    await user.click(outside);
    expect(screen.queryByText("Panel")).not.toBeInTheDocument();
    expect(outside).toHaveFocus();

    await user.click(trigger);
    await user.keyboard("{Escape}");
    expect(screen.queryByText("Panel")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByText("Panel")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("consumes Android back before a lower app handler", () => {
    const close = vi.fn();
    const lower = vi.fn();
    window.addEventListener("sshc-android-back", lower);
    render(<Layer name="menu" close={close} />);
    const back = new Event("sshc-android-back", { cancelable: true });

    void act(() => window.dispatchEvent(back));

    expect(back.defaultPrevented).toBe(true);
    expect(close).toHaveBeenCalledOnce();
    expect(lower).not.toHaveBeenCalled();
    window.removeEventListener("sshc-android-back", lower);
  });
});

it("keeps a parent modal open while a nested menu handles Android back", async () => {
  function Fixture() {
    const parent = useRef<HTMLDivElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const [parentOpen, setParentOpen] = useState(true);
    const [menuOpen, setMenuOpen] = useState(false);
    useDismissibleLayer({ open: parentOpen, containerRefs: [parent], trapFocus: true, onDismiss: () => setParentOpen(false) });
    useDismissibleLayer({ open: menuOpen, containerRefs: [menu], onDismiss: () => setMenuOpen(false) });
    return parentOpen ? <div ref={parent} role="dialog" aria-label="Parent">
      <button onClick={() => setMenuOpen(true)}>Open child menu</button>
      {menuOpen ? <div ref={menu} role="menu"><button role="menuitem">Child action</button></div> : null}
    </div> : null;
  }
  render(<Fixture />);
  await userEvent.click(screen.getByRole("button", { name: "Open child menu" }));
  expect(screen.getByRole("dialog", { name: "Parent" })).toBeInTheDocument();
  expect(screen.getByRole("menu")).toBeInTheDocument();
  act(() => { window.dispatchEvent(new Event("sshc-android-back", { cancelable: true })); });
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  expect(screen.getByRole("dialog", { name: "Parent" })).toBeInTheDocument();
  act(() => { window.dispatchEvent(new Event("sshc-android-back", { cancelable: true })); });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

describe("Escape and Tab inside a region that uses the keyboard itself", () => {
  // The SFTP editor's dialog: the editor is the dialog's last control.
  function EditorDialog({ close, keysTheEditorUses }: { close: () => void; keysTheEditorUses: readonly string[] }) {
    const panel = useRef<HTMLDivElement>(null);
    const editor = useRef<HTMLTextAreaElement>(null);
    useDismissibleLayer({ open: true, containerRefs: [panel], trapFocus: true, onDismiss: close });
    // Monaco と xterm は、自分で使ったキーを DOM の listener で止める。
    useEffect(() => {
      const input = editor.current;
      if (input === null) return;
      const stopKeyTheEditorUses = (event: KeyboardEvent) => {
        if (!keysTheEditorUses.includes(event.key)) return;
        event.preventDefault();
        event.stopPropagation();
      };
      input.addEventListener("keydown", stopKeyTheEditorUses);
      return () => input.removeEventListener("keydown", stopKeyTheEditorUses);
    }, [keysTheEditorUses]);
    return <>
      <div ref={panel} role="dialog" aria-label="Editor">
        <button>Close</button>
        <div {...keyboardOwnerProps}><textarea ref={editor} aria-label="Contents" /></div>
      </div>
      <button>Outside</button>
    </>;
  }

  const onlyEscape = ["Escape"];
  const onlyTab = ["Tab"];
  const noKeys: string[] = [];

  it("keeps the dialog open when the editor uses Escape", async () => {
    const close = vi.fn();
    const user = userEvent.setup();
    render(<EditorDialog close={close} keysTheEditorUses={onlyEscape} />);

    await user.click(screen.getByRole("textbox", { name: "Contents" }));
    await user.keyboard("{Escape}");

    expect(close).not.toHaveBeenCalled();
  });

  it("closes the dialog when the editor leaves Escape unused", async () => {
    const close = vi.fn();
    const user = userEvent.setup();
    render(<EditorDialog close={close} keysTheEditorUses={noKeys} />);

    await user.click(screen.getByRole("textbox", { name: "Contents" }));
    await user.keyboard("{Escape}");

    expect(close).toHaveBeenCalledWith("escape");
  });

  it("leaves focus in the editor when the editor uses Tab", async () => {
    const user = userEvent.setup();
    render(<EditorDialog close={vi.fn()} keysTheEditorUses={onlyTab} />);
    const contents = screen.getByRole("textbox", { name: "Contents" });

    await user.click(contents);
    await user.tab();
    expect(contents).toHaveFocus();

    await user.tab({ shift: true });
    expect(contents).toHaveFocus();
  });

  it("moves focus from the editor to the dialog's first control, not out of the dialog, when the editor leaves Tab unused", async () => {
    const user = userEvent.setup();
    render(<EditorDialog close={vi.fn()} keysTheEditorUses={noKeys} />);

    await user.click(screen.getByRole("textbox", { name: "Contents" }));
    await user.tab();

    expect(screen.getByRole("button", { name: "Close" })).toHaveFocus();
  });
});

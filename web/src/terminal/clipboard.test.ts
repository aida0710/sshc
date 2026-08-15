import { describe, expect, it, vi } from "vitest";
import { attachTerminalClipboard, type TerminalClipboardSettings } from "./clipboard";

function harness(initial: TerminalClipboardSettings = { copyOnSelect: true, rightClickPaste: true }) {
  const container = document.createElement("div");
  let settings = initial;
  let keyHandler: ((event: KeyboardEvent) => boolean) | undefined;
  let selectionHandler: () => void = () => {};
  let selection = "selected text";
  const terminal = {
    attachCustomKeyEventHandler: vi.fn((handler: (event: KeyboardEvent) => boolean) => {
      keyHandler = handler;
    }),
    onSelectionChange: vi.fn((handler: () => void) => {
      selectionHandler = handler;
      return { dispose: () => { selectionHandler = () => {}; } };
    }),
    hasSelection: vi.fn(() => selection !== ""),
    getSelection: vi.fn(() => selection),
    paste: vi.fn(),
  };
  const clipboard = {
    readText: vi.fn(async () => "pasted text"),
    writeText: vi.fn(async () => undefined),
  };
  const refuse = vi.fn();
  const detach = attachTerminalClipboard({
    container,
    terminal,
    clipboard,
    settings: () => settings,
    refuse,
  });
  return {
    container,
    terminal,
    clipboard,
    refuse,
    detach,
    key: (event: KeyboardEvent) => keyHandler?.(event),
    finishSelection: () => selectionHandler(),
    setSettings: (next: TerminalClipboardSettings) => {
      settings = next;
    },
    setSelection: (next: string) => {
      selection = next;
    },
  };
}

describe("terminal clipboard interactions", () => {
  it("copies once when xterm reports that selection is finished", () => {
    const subject = harness();

    subject.finishSelection();

    expect(subject.clipboard.writeText).toHaveBeenCalledTimes(1);
    expect(subject.clipboard.writeText).toHaveBeenCalledWith("selected text");
  });

  it("leaves selection and the context menu alone when each choice is off", () => {
    const subject = harness({ copyOnSelect: false, rightClickPaste: false });

    subject.finishSelection();
    const contextMenu = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    const allowed = subject.container.dispatchEvent(contextMenu);

    expect(subject.clipboard.writeText).not.toHaveBeenCalled();
    expect(subject.clipboard.readText).not.toHaveBeenCalled();
    expect(allowed).toBe(true);
  });

  it("reads the clipboard on right click and sends it through xterm paste", async () => {
    const subject = harness();
    const contextMenu = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });

    const allowed = subject.container.dispatchEvent(contextMenu);

    expect(allowed).toBe(false);
    await vi.waitFor(() => expect(subject.terminal.paste).toHaveBeenCalledWith("pasted text"));
  });

  it("uses xterm paste for the keyboard shortcut and observes settings changes without reattaching", async () => {
    const subject = harness();
    subject.setSettings({ copyOnSelect: false, rightClickPaste: false });

    expect(subject.key(new KeyboardEvent("keydown", { key: "v", metaKey: true }))).toBe(false);
    await vi.waitFor(() => expect(subject.terminal.paste).toHaveBeenCalledWith("pasted text"));
    subject.finishSelection();
    expect(subject.clipboard.writeText).not.toHaveBeenCalled();
  });

  it("removes DOM handlers when the terminal is disposed", () => {
    const subject = harness();
    subject.detach();

    subject.finishSelection();
    subject.container.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true }));

    expect(subject.clipboard.writeText).not.toHaveBeenCalled();
    expect(subject.clipboard.readText).not.toHaveBeenCalled();
  });
});

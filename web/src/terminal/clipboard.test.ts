import { describe, expect, it, vi } from "vitest";
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";

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
  it("normalizes and brackets paste text before sending it to a terminal", () => {
    expect(prepareTerminalPaste("one\ntwo\r\nthree", false)).toBe("one\rtwo\rthree");
    expect(prepareTerminalPaste("one\ntwo", true)).toBe("\u001b[200~one\rtwo\u001b[201~");
  });
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

  it("leaves keyboard paste to the browser paste event", () => {
    const subject = harness();
    subject.setSettings({ copyOnSelect: false, rightClickPaste: false });

    const shortcut = new KeyboardEvent("keydown", {
      key: "v",
      ctrlKey: true,
      shiftKey: true,
      cancelable: true,
    });
    expect(subject.key(shortcut)).toBe(false);
    expect(shortcut.defaultPrevented).toBe(false);
    expect(subject.clipboard.readText).not.toHaveBeenCalled();
    expect(subject.terminal.paste).not.toHaveBeenCalled();
    subject.finishSelection();
    expect(subject.clipboard.writeText).not.toHaveBeenCalled();
  });

  it("owns a bubbling browser paste once before xterm's nested listeners", () => {
    const subject = harness();
    const textarea = document.createElement("textarea");
    subject.container.append(textarea);
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(paste, "clipboardData", {
      value: { getData: (format: string) => format === "text/plain" ? "one paste" : "" },
    });

    textarea.dispatchEvent(paste);

    expect(paste.defaultPrevented).toBe(true);
    expect(subject.terminal.paste).toHaveBeenCalledTimes(1);
    expect(subject.terminal.paste).toHaveBeenCalledWith("one paste");
  });

  it("sends an application-enabled enhanced key once instead of xterm input", () => {
    const container = document.createElement("div");
    let keyHandler: ((event: KeyboardEvent) => boolean) | undefined;
    const sendEnhancedKey = vi.fn();
    attachTerminalClipboard({
      container,
      terminal: {
        attachCustomKeyEventHandler: (handler) => { keyHandler = handler; },
        onSelectionChange: () => ({ dispose: () => {} }),
        hasSelection: () => false,
        getSelection: () => "",
        paste: vi.fn(),
      },
      clipboard: { readText: vi.fn(), writeText: vi.fn() },
      settings: () => ({ copyOnSelect: false, rightClickPaste: false }),
      refuse: vi.fn(),
      enhancedKey: () => "\u001b[13;2u",
      sendEnhancedKey,
    });
    const event = new KeyboardEvent("keydown", { key: "Enter", shiftKey: true });
    expect(keyHandler?.(event)).toBe(false);
    expect(sendEnhancedKey).toHaveBeenCalledWith("\u001b[13;2u");
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

describe("the long press is not a right click", () => {
  function stub() {
    return {
      attachCustomKeyEventHandler: vi.fn(),
      onSelectionChange: vi.fn(() => ({ dispose: () => {} })),
      hasSelection: vi.fn(() => false),
      getSelection: vi.fn(() => ""),
      paste: vi.fn(),
    };
  }

  it("leaves the context menu alone where the pointer is coarse", () => {
    const container = document.createElement("div");
    const readText = vi.fn(async () => "pasted text");
    const detach = attachTerminalClipboard({
      container,
      terminal: stub(),
      clipboard: { readText, writeText: vi.fn(async () => undefined) },
      coarsePointer: () => true,
      settings: () => ({ copyOnSelect: true, rightClickPaste: true }),
      refuse: vi.fn(),
    });

    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    container.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(readText).not.toHaveBeenCalled();
    detach();
  });

  it("still pastes on a right click where a mouse exists", () => {
    const container = document.createElement("div");
    const readText = vi.fn(async () => "pasted text");
    const detach = attachTerminalClipboard({
      container,
      terminal: stub(),
      clipboard: { readText, writeText: vi.fn(async () => undefined) },
      coarsePointer: () => false,
      settings: () => ({ copyOnSelect: true, rightClickPaste: true }),
      refuse: vi.fn(),
    });

    const event = new MouseEvent("contextmenu", { bubbles: true, cancelable: true });
    container.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(readText).toHaveBeenCalled();
    detach();
  });
});

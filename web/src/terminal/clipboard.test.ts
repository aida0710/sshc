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

  // **触れる画面に右のボタンは無い。** Android は長押しで contextmenu を出す
  // ので、そこで既定を止めると OS がまさに始めようとしていた範囲選択ごと消える。
  // 実機ではそれが起きていて、ついでに読み取れないクリップボードを読みに行き、
  // 「アクセスできませんでした」を出していた。
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

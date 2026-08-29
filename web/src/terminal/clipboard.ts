export type TerminalClipboardSettings = {
  copyOnSelect: boolean;
  rightClickPaste: boolean;
};

type ClipboardTerminal = {
  attachCustomKeyEventHandler(handler: (event: KeyboardEvent) => boolean): void;
  onSelectionChange(handler: () => void): { dispose(): void };
  hasSelection(): boolean;
  getSelection(): string;
  paste(text: string): void;
};

type ClipboardAccess = Pick<Clipboard, "readText" | "writeText">;

type TerminalClipboardOptions = {
  container: HTMLElement;
  terminal: ClipboardTerminal;
  paste?: (text: string) => void;
  clipboard: ClipboardAccess;
  coarsePointer?: () => boolean;
  settings: () => TerminalClipboardSettings;
  refuse: () => void;
  enhancedKey?: (event: KeyboardEvent) => string | null;
  sendEnhancedKey?: (sequence: string) => void;
};

export function prepareTerminalPaste(text: string, bracketed: boolean): string {
  const normalized = text.replace(/\r?\n/g, "\r");
  return bracketed ? `\u001b[200~${normalized}\u001b[201~` : normalized;
}

export function attachTerminalClipboard({
  container,
  terminal,
  paste: pasteText = (text) => terminal.paste(text),
  clipboard,
  coarsePointer = () => false,
  settings,
  refuse,
  enhancedKey,
  sendEnhancedKey,
}: TerminalClipboardOptions): () => void {
  const copySelection = () => {
    if (!terminal.hasSelection()) return;
    const text = terminal.getSelection();
    if (text === "") return;
    void clipboard.writeText(text).catch(refuse);
  };

  const readAndPaste = () => {
    void clipboard
      .readText()
      .then((text) => {
        if (text !== "") pasteText(text);
      })
      .catch(refuse);
  };

  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== "keydown") return true;
    const sequence = enhancedKey?.(event) ?? null;
    if (sequence !== null) {
      sendEnhancedKey?.(sequence);
      return false;
    }
    if (!event.metaKey && !(event.ctrlKey && event.shiftKey)) return true;
    const key = event.key.toLowerCase();
    if (key === "c" && terminal.hasSelection()) {
      copySelection();
      return false;
    }
    if (key === "v") {
      // Let the browser emit its normal paste event. The capture handler below
      // owns that event before xterm's nested listeners see it. Returning false
      // keeps xterm from also translating Ctrl+V into terminal input.
      return false;
    }
    return true;
  });

  const selection = terminal.onSelectionChange(() => {
    if (!settings().copyOnSelect) return;
    copySelection();
  });
  const onPaste = (event: ClipboardEvent) => {
    const text = event.clipboardData?.getData("text/plain");
    if (text === undefined) return;
    // Own the event once at xterm's parent so neither its nested listeners nor
    // a custom keyboard shortcut can duplicate terminal input.
    event.preventDefault();
    event.stopImmediatePropagation();
    if (text !== "") pasteText(text);
  };
  const onContextMenu = (event: MouseEvent) => {
    if (coarsePointer()) return;
    if (!settings().rightClickPaste) return;
    event.preventDefault();
    readAndPaste();
  };
  container.addEventListener("paste", onPaste, { capture: true });
  container.addEventListener("contextmenu", onContextMenu);

  return () => {
    selection.dispose();
    container.removeEventListener("paste", onPaste, { capture: true });
    container.removeEventListener("contextmenu", onContextMenu);
  };
}

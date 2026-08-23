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
  clipboard: ClipboardAccess;
  coarsePointer?: () => boolean;
  settings: () => TerminalClipboardSettings;
  refuse: () => void;
};

export function attachTerminalClipboard({
  container,
  terminal,
  clipboard,
  coarsePointer = () => false,
  settings,
  refuse,
}: TerminalClipboardOptions): () => void {
  const copySelection = () => {
    if (!terminal.hasSelection()) return;
    const text = terminal.getSelection();
    if (text === "") return;
    void clipboard.writeText(text).catch(refuse);
  };

  const paste = () => {
    void clipboard
      .readText()
      .then((text) => {
        if (text !== "") terminal.paste(text);
      })
      .catch(refuse);
  };

  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== "keydown") return true;
    if (!event.metaKey && !(event.ctrlKey && event.shiftKey)) return true;
    const key = event.key.toLowerCase();
    if (key === "c" && terminal.hasSelection()) {
      copySelection();
      return false;
    }
    if (key === "v") {
      paste();
      return false;
    }
    return true;
  });

  const selection = terminal.onSelectionChange(() => {
    if (!settings().copyOnSelect) return;
    copySelection();
  });
  const onContextMenu = (event: MouseEvent) => {
    if (coarsePointer()) return;
    if (!settings().rightClickPaste) return;
    event.preventDefault();
    paste();
  };
  container.addEventListener("contextmenu", onContextMenu);

  return () => {
    selection.dispose();
    container.removeEventListener("contextmenu", onContextMenu);
  };
}

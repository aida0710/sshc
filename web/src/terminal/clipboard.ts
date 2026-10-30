import type { PastedItem } from "./kittyClipboard";
import { matchesShortcut } from "../keyconfig/bindings";
import { currentBindings } from "../keyconfig/presets";
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
  // nothingToPaste は、貼り付けに文字が無かったときに呼ぶ。image は、クリップボードにあったのが
  // 画像だけだったか（ターミナルへ送れるのは文字だけである）。黙っていると、貼り付けが
  // 効かなかったのか、何も起きなかったのかが分からない。
  nothingToPaste?: (image: boolean) => void;
  // takePaste hands a paste, in every form the clipboard gave it, to a program
  // that asked for paste events (OSC 5522, kittyClipboard.ts), and says
  // whether it did; then nothing is typed. The forms are gathered only then.
  takePaste?: (items: () => PastedItem[]) => boolean;
  // copied は、コピーのショートカットで選択を写せたときだけ呼ぶ。選ぶだけで写す設定のときに
  // 毎回知らせると、選び直すたびに知らせが出てうるさい。
  copied?: () => void;
  enhancedKey?: (event: KeyboardEvent) => string | null;
  sendEnhancedKey?: (sequence: string) => void;
  // selectionIsSearchMatch is true while the scrollback search selects the
  // match it moved to. Copying that would replace the clipboard with the query.
  selectionIsSearchMatch?: () => boolean;
};

// pastedItems is every form a paste holds: the text forms as text, the files
// (an image) as they are.
function pastedItems(data: DataTransfer): PastedItem[] {
  return [...data.items].flatMap((item): PastedItem[] => {
    if (item.kind === "string") return [{ mime: item.type, data: data.getData(item.type) }];
    const file = item.getAsFile();
    return file === null ? [] : [{ mime: item.type || file.type, data: file }];
  });
}

export function prepareTerminalPaste(text: string, bracketed: boolean): string {
  const normalized = text.replace(/\r?\n/g, "\r");
  if (!bracketed) return normalized;
  // An ESC inside would let the text end the bracket early (ESC [201~) and
  // have the rest typed. xterm.js shows it as ␛ the same way.
  return `\u001b[200~${normalized.replace(/\u001b/g, "\u241b")}\u001b[201~`;
}

export function attachTerminalClipboard({
  container,
  terminal,
  paste: pasteText = (text) => terminal.paste(text),
  clipboard,
  coarsePointer = () => false,
  settings,
  refuse,
  nothingToPaste,
  takePaste = () => false,
  copied,
  enhancedKey,
  sendEnhancedKey,
  selectionIsSearchMatch = () => false,
}: TerminalClipboardOptions): () => void {
  let active = true;
  const copySelection = ({ announce }: { announce: boolean }) => {
    if (!terminal.hasSelection()) return;
    const text = terminal.getSelection();
    if (text === "") return;
    void clipboard.writeText(text).then(() => {
      if (announce && active) copied?.();
    }, refuse);
  };

  const readAndPaste = () => {
    void clipboard
      .readText()
      .then((text) => {
        // Clipboard reads can outlive the terminal effect which started them.
        // Never deliver an old session's paste into a replacement session.
        if (!active) return;
        // readText cannot tell an image from an empty clipboard; the paste event below can.
        if (text === "") nothingToPaste?.(false);
        else if (!takePaste(() => [{ mime: "text/plain", data: text }])) pasteText(text);
      })
      .catch(() => {
        if (active) refuse();
      });
  };

  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== "keydown") return true;
    if (matchesShortcut(event, "copy", currentBindings()) && terminal.hasSelection()) {
      event.preventDefault();
      copySelection({ announce: true });
      return false;
    }
    if (matchesShortcut(event, "paste", currentBindings())) {
      const nativePaste = !event.altKey && ((event.metaKey && !event.ctrlKey && !event.shiftKey) || (event.ctrlKey && !event.metaKey)) && event.key.toLowerCase() === "v";
      if (!nativePaste) {
        event.preventDefault();
        if (!event.repeat) readAndPaste();
      }
      // Native chords are delivered by the browser paste event exactly once.
      return false;
    }
    const sequence = enhancedKey?.(event) ?? null;
    if (sequence !== null) {
      // Returning false skips xterm's cancellation too: consume the browser
      // action so enhanced keys such as Shift+Tab cannot also move focus.
      event.preventDefault();
      event.stopPropagation();
      sendEnhancedKey?.(sequence);
      return false;
    }
    return true;
  });

  const selection = terminal.onSelectionChange(() => {
    if (!settings().copyOnSelect || selectionIsSearchMatch()) return;
    copySelection({ announce: false });
  });
  const onPaste = (event: ClipboardEvent) => {
    const data = event.clipboardData;
    if (data === null) return;
    // Own the event once at xterm's parent so neither its nested listeners nor
    // a custom keyboard shortcut can duplicate terminal input.
    event.preventDefault();
    event.stopImmediatePropagation();
    if (takePaste(() => pastedItems(data))) return;
    const text = data.getData("text/plain");
    if (text !== "") {
      pasteText(text);
      return;
    }
    nothingToPaste?.([...data.items].some((item) => item.kind === "file" && item.type.startsWith("image/")));
  };
  // A keyboard's clipboard suggestion can arrive as an input event instead of
  // a paste event, and xterm.js would send its text as typed. Text that would
  // run lines goes through the same review as a paste.
  const onBeforeInput = (event: InputEvent) => {
    const text = event.data ?? event.dataTransfer?.getData("text/plain") ?? "";
    const pasted = event.inputType === "insertFromPaste" || (event.inputType === "insertText" && /[\r\n]/u.test(text));
    if (!pasted || text === "") return;
    event.preventDefault();
    event.stopImmediatePropagation();
    pasteText(text);
  };
  const onContextMenu = (event: MouseEvent) => {
    if (coarsePointer()) return;
    if (!settings().rightClickPaste) return;
    event.preventDefault();
    readAndPaste();
  };
  container.addEventListener("paste", onPaste, { capture: true });
  container.addEventListener("beforeinput", onBeforeInput, { capture: true });
  container.addEventListener("contextmenu", onContextMenu);

  return () => {
    active = false;
    selection.dispose();
    container.removeEventListener("paste", onPaste, { capture: true });
    container.removeEventListener("beforeinput", onBeforeInput, { capture: true });
    container.removeEventListener("contextmenu", onContextMenu);
  };
}

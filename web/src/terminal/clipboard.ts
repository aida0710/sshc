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
  // coarsePointer は、指で触る画面かどうかを答える。既定は「マウスがある」。
  coarsePointer?: () => boolean;
  settings: () => TerminalClipboardSettings;
  refuse: () => void;
};

// attachTerminalClipboard は、xterm の選択と貼り付けをブラウザのクリップ
// ボードへ繋ぐ。設定はイベントのたびに読むので、開いている端末を作り直さずに
// on/off を反映できる。
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

  // xterm.paste を通す。PTY へ直接送ると、xterm が bracketed paste mode に
  // 合わせて付ける開始・終了マーカーを迂回してしまう。
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

  // xterm はドラッグ中ではなく、document の mouseup で選択を確定してから
  // onSelectionChange を発火する。端末要素の mouseup を見ると、端末の外まで
  // 引きずった選択だけ取りこぼす。
  const selection = terminal.onSelectionChange(() => {
    if (!settings().copyOnSelect) return;
    copySelection();
  });
  const onContextMenu = (event: MouseEvent) => {
    // **長押しは右クリックではない。** 触れる画面に右のボタンは無く、Android は
    // 長押しで contextmenu を出す。ここで既定を止めると、OS がまさに始めようと
    // していた範囲選択ごと消える——実機ではそれが起きていた。ついでに、読み取れ
    // ないクリップボードを読みに行って「アクセスできませんでした」を出していた。
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

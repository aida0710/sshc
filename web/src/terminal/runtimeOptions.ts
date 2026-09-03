export type MutableTerminalRuntimeOptions = {
  cursorBlink?: boolean;
  fontSize?: number;
  scrollback?: number;
};

export type TerminalRuntimeSettings = {
  cursorBlink: boolean;
  fontSize: number;
  scrollback: number;
};

// xterm keeps these options live. Updating the React settings without copying
// them into the existing terminal leaves an open pane on its old values until
// the component is recreated.
export function applyTerminalRuntimeOptions(
  options: MutableTerminalRuntimeOptions,
  settings: TerminalRuntimeSettings,
): { refit: boolean } {
  const refit = options.fontSize !== settings.fontSize;
  options.cursorBlink = settings.cursorBlink;
  options.fontSize = settings.fontSize;
  options.scrollback = settings.scrollback;
  return { refit };
}

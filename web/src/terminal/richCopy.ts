type SelectableTerminal = { hasSelection(): boolean; getSelection(): string };
type HTMLSerializer = { serializeAsHTML(options: { onlySelection: boolean; includeGlobalBackground: boolean }): string };

// selectionWithColors is what "Copy with colors" puts on the clipboard: the
// terminal's selection as plain text, and as HTML that keeps its colours,
// bold, italics and underline on the terminal's own background, so a rich
// editor shows it as the terminal did. OSC 8 links stay as their text:
// xterm.js does not tell which cell links where. Null when nothing is
// selected.
export function selectionWithColors(view: SelectableTerminal, serializer: HTMLSerializer): { text: string; html: string } | null {
  if (!view.hasSelection()) return null;
  const text = view.getSelection();
  if (text === "") return null;
  return { text, html: serializer.serializeAsHTML({ onlySelection: true, includeGlobalBackground: true }) };
}

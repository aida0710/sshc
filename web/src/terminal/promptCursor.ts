import type { IBuffer, IMarker, IParser } from "@xterm/xterm";
import type { BufferCell } from "./tappedLink";

type PromptTerminal = {
  readonly parser: Pick<IParser, "registerOscHandler">;
  readonly buffer: { readonly active: IBuffer };
  readonly modes: { readonly applicationCursorKeysMode: boolean; readonly mouseTrackingMode: string };
  registerMarker(cursorYOffset?: number): IMarker | undefined;
};

// A click on the line being typed at a prompt moves the shell's cursor there,
// as Ghostty's cursor-click-to-move does: the click becomes one arrow key for
// each character between the cursor and the cell. The shell reports where the
// typed text begins with OSC 133;B, so this works only with shell integration,
// and only while the typed text fits on one line: in an input of several lines
// the arrow keys move in ways that depend on the shell.
export type PromptCursor = {
  // arrowsTo is the arrow keys that take the cursor to cell, or null when the
  // cell is not in the text being typed at a prompt.
  arrowsTo(cell: BufferCell): string | null;
  dispose(): void;
};

export function trackPromptCursor(view: PromptTerminal): PromptCursor {
  let input: { marker: IMarker; start: number } | null = null;
  const forget = () => {
    input?.marker.dispose();
    input = null;
  };
  const watcher = view.parser.registerOscHandler(133, (data) => {
    const [kind] = data.split(";", 1);
    if (kind === "B") {
      forget();
      // The parser is where the shell's output is, so the cursor is where typing starts.
      const marker = view.registerMarker(0);
      if (marker !== undefined) input = { marker, start: view.buffer.active.cursorX };
    } else if (kind === "A" || kind === "C" || kind === "D") {
      forget();
    }
    // The command marks (osc133.ts) consume the sequence; this only watches it.
    return false;
  });

  return {
    arrowsTo(cell) {
      const buffer = view.buffer.active;
      // A program reading the mouse gets the click itself; one on the
      // alternate screen is not at the prompt.
      if (input === null || input.marker.isDisposed || buffer.type !== "normal" || view.modes.mouseTrackingMode !== "none") return null;
      const line = buffer.getLine(cell.y);
      const typingLine = buffer.baseY + buffer.cursorY;
      if (line === undefined || cell.y !== input.marker.line || cell.y !== typingLine || cell.x < input.start) return null;
      if (buffer.getLine(cell.y + 1)?.isWrapped === true) return null;
      const target = Math.min(characterStart(line, cell.x), Math.max(textEnd(line, input.start), buffer.cursorX));
      return arrows(line, buffer.cursorX, target, view.modes.applicationCursorKeysMode);
    },
    dispose() {
      watcher.dispose();
      forget();
    },
  };
}

type BufferLine = NonNullable<ReturnType<IBuffer["getLine"]>>;

// characterStart steps back from the right half of a wide character to the
// cell where the character is.
function characterStart(line: BufferLine, x: number): number {
  return x > 0 && line.getCell(x)?.getWidth() === 0 ? x - 1 : x;
}

// textEnd is the cell after the last character on the line from start on.
function textEnd(line: BufferLine, start: number): number {
  for (let x = line.length - 1; x >= start; x -= 1) {
    const character = line.getCell(x);
    if (character !== undefined && character.getChars() !== "") return x + character.getWidth();
  }
  return start;
}

// arrows counts characters, not cells, between the cursor and target: a wide
// character takes two cells and one arrow key.
function arrows(line: BufferLine, from: number, to: number, applicationCursor: boolean): string | null {
  let characters = 0;
  for (let x = Math.min(from, to); x < Math.max(from, to); x += 1) {
    if (line.getCell(x)?.getWidth() !== 0) characters += 1;
  }
  if (characters === 0) return null;
  const key = to > from ? (applicationCursor ? "\u001bOC" : "\u001b[C") : (applicationCursor ? "\u001bOD" : "\u001b[D");
  return key.repeat(characters);
}

// attachPromptCursorClicks moves the cursor on a plain click of the mouse. A
// touch screen reports taps through the selection overlay instead, so clicks
// are ignored while enabled says no.
export function attachPromptCursorClicks({
  container,
  prompt,
  cellAt,
  hasSelection,
  enabled,
  send,
}: {
  container: HTMLElement;
  prompt: PromptCursor;
  cellAt: (event: MouseEvent) => BufferCell | null;
  hasSelection: () => boolean;
  enabled: () => boolean;
  send: (keys: string) => void;
}): () => void {
  const clicked = (event: MouseEvent) => {
    // A double click selects a word and a drag selects text; modifiers keep
    // their own meaning (Alt+click is xterm.js's own cursor move).
    if (!enabled() || event.button !== 0 || event.detail !== 1 || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || hasSelection()) return;
    const cell = cellAt(event);
    const keys = cell === null ? null : prompt.arrowsTo(cell);
    if (keys !== null) send(keys);
  };
  container.addEventListener("click", clicked);
  return () => container.removeEventListener("click", clicked);
}

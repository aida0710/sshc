import type { Terminal } from "@xterm/xterm";

// resizeRewritingCursorLine resizes the terminal and, when its width changed, writes the line holding the cursor again
// at the new width, as far as the cursor.
//
// xterm.js leaves the line holding the cursor as it is when the width changes (its reflowCursorLine is off), because a
// shell draws its prompt again on the new size. Nothing draws again the questions sshc's engine asks while it connects
// (a host key, a password): a session starts at 80 columns, so on a phone of 40 columns the end of "… continue
// connecting (yes/no)? " was cut off and the answer typed over its last letter. A question has nothing after the
// cursor, so the line up to the cursor is all there is to write.
export function resizeRewritingCursorLine(view: Terminal, resize: () => void): void {
  const columns = view.cols;
  const text = textBeforeCursor(view);
  resize();
  if (view.cols === columns || text === "") return;
  const rows = cursorLineRows(view);
  // Back to the first row of the line, every one of its rows cleared (not the screen below it), then the text.
  const up = (count: number) => (count > 0 ? `\x1b[${count}A` : "");
  const clear = Array.from({ length: rows }, () => "\x1b[2K").join("\x1b[B");
  view.write(`\r${up(rows - 1)}${clear}\r${up(rows - 1)}${text}`);
}

// textBeforeCursor is the line holding the cursor (with the rows it wrapped from) up to the cursor.
function textBeforeCursor(view: Terminal): string {
  const buffer = view.buffer.active;
  const cursorRow = buffer.baseY + buffer.cursorY;
  const first = firstRowOfLine(view, cursorRow);
  let text = "";
  for (let row = first; row < cursorRow; row += 1) text += buffer.getLine(row)?.translateToString(false) ?? "";
  return text + (buffer.getLine(cursorRow)?.translateToString(false, 0, buffer.cursorX) ?? "");
}

// cursorLineRows is how many rows the line holding the cursor takes, up to the cursor.
function cursorLineRows(view: Terminal): number {
  const buffer = view.buffer.active;
  const cursorRow = buffer.baseY + buffer.cursorY;
  return cursorRow - firstRowOfLine(view, cursorRow) + 1;
}

function firstRowOfLine(view: Terminal, row: number): number {
  const buffer = view.buffer.active;
  let first = row;
  while (first > 0 && buffer.getLine(first)?.isWrapped === true) first -= 1;
  return first;
}

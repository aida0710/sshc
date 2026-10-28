import { recentBufferText, type ReadableBuffer } from "./buffer";

// The terminal's whole buffer, its scrollback and its screen, saved as a text
// file: "Copy recent terminal context" stops at 200 rows, this does not.

// scrollbackText is every line of the buffer, with wrapped rows joined into
// the lines they were written as and the empty rows at the end left out.
export function scrollbackText(buffer: ReadableBuffer): string {
  return recentBufferText(buffer, buffer.length, Number.POSITIVE_INFINITY);
}

// Characters a file name cannot hold on Windows, macOS or Android's storage.
const unsafeFileNameCharacters = /[\\/:*?"<>|\u0000-\u001f\u007f]+/g;
// A pane name can be a whole command line; the file name keeps its start.
const maximumTitleInFileName = 80;

// scrollbackFileName names the file after the pane and the local time it was saved, so that saves of one
// session sort in order.
export function scrollbackFileName(title: string, savedAt: Date): string {
  const name = title.replace(unsafeFileNameCharacters, "_").trim().slice(0, maximumTitleInFileName) || "terminal";
  const two = (value: number) => String(value).padStart(2, "0");
  const date = `${savedAt.getFullYear()}${two(savedAt.getMonth() + 1)}${two(savedAt.getDate())}`;
  const time = `${two(savedAt.getHours())}${two(savedAt.getMinutes())}${two(savedAt.getSeconds())}`;
  return `${name}-${date}-${time}.txt`;
}

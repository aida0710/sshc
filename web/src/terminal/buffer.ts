
export type BufferLine = {
  isWrapped?: boolean;
  translateToString(trimRight?: boolean, startColumn?: number, endColumn?: number): string;
};
export type ReadableBuffer = { length: number; getLine(index: number): BufferLine | undefined };
export type ViewportBuffer = ReadableBuffer & { viewportY: number };

const contextControlCharacters = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/g;

export function recentBufferText(buffer: ReadableBuffer, maxRows = 200, maxCharacters = 65_536): string {
  if (maxRows <= 0 || maxCharacters <= 0) return "";
  const start = Math.max(0, buffer.length - maxRows);
  const logicalLines: string[] = [];
  for (let row = start; row < buffer.length; row += 1) {
    const line = buffer.getLine(row);
    if (line === undefined) continue;
    const text = line.translateToString(true).replace(contextControlCharacters, "");
    if (line.isWrapped === true && logicalLines.length > 0) {
      logicalLines[logicalLines.length - 1] += text;
    } else {
      logicalLines.push(text);
    }
  }
  while (logicalLines.at(-1) === "") logicalLines.pop();
  const joined = logicalLines.join("\n");
  return joined.length <= maxCharacters ? joined : joined.slice(joined.length - maxCharacters);
}

export function viewportText(buffer: ViewportBuffer, rows: number, cols: number): string {
  const lines: string[] = [];
  for (let row = 0; row < rows; row += 1) {
    lines.push(buffer.getLine(buffer.viewportY + row)?.translateToString(true, 0, cols) ?? "");
  }
  return lines.join("\n");
}

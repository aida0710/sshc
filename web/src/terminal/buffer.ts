
export type BufferLine = {
  translateToString(trimRight?: boolean, startColumn?: number, endColumn?: number): string;
};
export type ReadableBuffer = { length: number; getLine(index: number): BufferLine | undefined };
export type ViewportBuffer = ReadableBuffer & { viewportY: number };
export function viewportText(buffer: ViewportBuffer, rows: number, cols: number): string {
  const lines: string[] = [];
  for (let row = 0; row < rows; row += 1) {
    lines.push(buffer.getLine(buffer.viewportY + row)?.translateToString(true, 0, cols) ?? "");
  }
  return lines.join("\n");
}

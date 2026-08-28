
function offsetOfLine(contents: string, line: number): number {
  let offset = 0;
  for (let current = 1; current < line; current += 1) {
    const next = contents.indexOf("\n", offset);
    if (next < 0) throw new Error("block_moved");
    offset = next + 1;
  }
  return offset;
}

function commentOffset(contents: string, offset: number, commentLines: number): number {
  let start = offset;
  for (let remaining = commentLines; remaining > 0; remaining--) {
    const previous = contents.lastIndexOf("\n", start - 2);
    start = previous < 0 ? 0 : previous + 1;
  }
  return start;
}

export function removeHostBlock(
  contents: string,
  line: number,
  raw: string,
  commentLines = 0,
): string {
  const offset = offsetOfLine(contents, line);
  if (!contents.startsWith(raw, offset)) throw new Error("block_moved");
  const start = commentOffset(contents, offset, commentLines);
  return contents.slice(0, start) + contents.slice(offset + raw.length);
}

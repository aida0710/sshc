
function offsetOfLine(contents: string, line: number): number {
  let offset = 0;
  for (let current = 1; current < line; current += 1) {
    const next = contents.indexOf("\n", offset);
    if (next < 0) throw new Error("block_moved");
    offset = next + 1;
  }
  return offset;
}

export function duplicateHostBlock(
  contents: string,
  raw: string,
  alias: string,
  newAlias: string,
  line = 0,
  commentLines = 0,
): string {
  const lineBreak = raw.indexOf("\n");
  const header = lineBreak < 0 ? raw : raw.slice(0, lineBreak);
  const rest = lineBreak < 0 ? "" : raw.slice(lineBreak);
  const tokens = header.split(" ");
  const aliasIndex = tokens.indexOf(alias);
  if (aliasIndex < 0) throw new Error("block_moved");
  tokens[aliasIndex] = newAlias;
  const copied = `${tokens.join(" ")}${rest}`;
  let comment = "";
  if (commentLines > 0 && line > 0) {
    const offset = offsetOfLine(contents, line);
    comment = contents.slice(commentOffset(contents, offset, commentLines), offset);
  }
  const terminated = contents.endsWith("\n") ? contents : `${contents}\n`;
  return `${terminated}\n${comment}${copied.endsWith("\n") ? copied : `${copied}\n`}`;
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

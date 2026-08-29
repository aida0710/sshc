export type BufferMatch = { row: number; column: number; length: number };

export function findBufferMatches(lines: string[], query: string): BufferMatch[] {
  if (query === "") return [];
  const needle = query.toLocaleLowerCase();
  const matches: BufferMatch[] = [];
  lines.forEach((line, row) => {
    const haystack = line.toLocaleLowerCase();
    let from = 0;
    while (from <= haystack.length - needle.length) {
      const column = haystack.indexOf(needle, from);
      if (column < 0) break;
      matches.push({ row, column, length: query.length });
      from = column + Math.max(1, needle.length);
    }
  });
  return matches;
}

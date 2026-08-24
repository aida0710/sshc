export type DraftUpdate = { draft: string; completed: string[] };

export function updateCommandDraft(current: string, data: string): DraftUpdate {
  let draft = current;
  const completed: string[] = [];
  for (const character of data) {
    if (character === "\r" || character === "\n") {
      const command = draft.trim();
      if (command !== "") completed.push(command);
      draft = "";
      continue;
    }
    if (character === "\u007f" || character === "\b") {
      draft = [...draft].slice(0, -1).join("");
      continue;
    }
    if (character === "\u0003" || character === "\u0015" || character === "\u001b") {
      draft = "";
      continue;
    }
    if (character >= " " && character !== "\u007f") draft += character;
  }
  return { draft, completed };
}

export function frequentCommandSuggestions(history: string[], draft: string, limit = 5): string[] {
  const prefix = draft.trimStart();
  if (prefix === "" || prefix.includes("\n")) return [];
  const ranked = new Map<string, { count: number; recent: number }>();
  history.forEach((command, index) => {
    if (command === prefix || !command.startsWith(prefix)) return;
    const current = ranked.get(command);
    ranked.set(command, { count: (current?.count ?? 0) + 1, recent: index });
  });
  return [...ranked]
    .sort((left, right) => right[1].count - left[1].count || right[1].recent - left[1].recent)
    .slice(0, limit)
    .map(([command]) => command);
}

export type AbsolutePathDraft = { token: string; parent: string; basename: string };

export function absolutePathDraft(draft: string): AbsolutePathDraft | null {
  const match = draft.match(/(?:^|\s)(\/[^\s]*)$/);
  const token = match?.[1];
  if (token === undefined) return null;
  const slash = token.lastIndexOf("/");
  return {
    token,
    parent: slash === 0 ? "/" : token.slice(0, slash),
    basename: token.slice(slash + 1),
  };
}

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

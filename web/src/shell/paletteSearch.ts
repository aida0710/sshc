// The palette's search. An item is found when every word typed is in its name, details or keywords; the items
// found keep their kinds in order (commands, connections, sessions, files, …), and within a kind the ones whose
// name is closer to what was typed come first: "web" finds web-01 before osaka-web-01.

export type SearchableItem = {
  kind: string;
  // name is what the person calls the item (a connection's alias, a session's title); it decides the order.
  name: string;
  label: string;
  detail: string;
  search: string;
};

function words(query: string): string[] {
  return query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
}

function found(item: SearchableItem, tokens: string[]): boolean {
  const haystack = `${item.label} ${item.detail} ${item.search}`.toLocaleLowerCase();
  return tokens.every((token) => haystack.includes(token));
}

// closeness is 0 when the name is what was typed, 1 when it starts with it, 2 when one of its words (split at
// - _ . / and spaces) does, and 3 when the match is elsewhere.
function closeness(item: SearchableItem, typed: string): number {
  const name = item.name.toLocaleLowerCase();
  if (name === typed) return 0;
  if (name.startsWith(typed)) return 1;
  if (name.split(/[-_./\s]+/u).some((part) => part.startsWith(typed))) return 2;
  return 3;
}

export function searchPalette<T extends SearchableItem>(items: readonly T[], query: string): T[] {
  const tokens = words(query);
  if (tokens.length === 0) return [...items];
  const typed = tokens.join(" ");
  const kindOrder = new Map<string, number>();
  for (const item of items) if (!kindOrder.has(item.kind)) kindOrder.set(item.kind, kindOrder.size);
  return items
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => found(item, tokens))
    .map((entry) => ({ ...entry, closeness: closeness(entry.item, typed) }))
    .sort((a, b) => kindOrder.get(a.item.kind)! - kindOrder.get(b.item.kind)! || a.closeness - b.closeness || a.index - b.index)
    .map(({ item }) => item);
}

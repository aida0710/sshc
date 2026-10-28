// encodeIntlYen sends the yen key of a JIS keyboard as a backslash (Shift: a
// vertical bar) when the setting asks for it: Japanese layouts put the
// backslash that shells and programming languages need on that key.
export function encodeIntlYen(event: KeyboardEvent, enabled: boolean): string | null {
  if (!enabled || event.type !== "keydown" || event.isComposing || event.code !== "IntlYen") return null;
  if (event.ctrlKey || event.altKey || event.metaKey) return null;
  return event.shiftKey ? "|" : "\\";
}

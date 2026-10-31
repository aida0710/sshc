export type TerminalSearchSettings = {
  caseSensitive: boolean;
  regex: boolean;
};

export function validSearchPattern(query: string, settings: TerminalSearchSettings): boolean {
  if (!settings.regex || query === "") return true;
  try {
    new RegExp(query, settings.caseSensitive ? "g" : "gi");
    return true;
  } catch {
    return false;
  }
}

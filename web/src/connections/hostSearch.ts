// The one rule for "does this host match what was typed": a case-insensitive
// substring over every field a person might remember the host by. Screens
// differ in which fields they have, not in how they match.
export type HostSearchFields = {
  alias: string;
  group?: string;
  hostName?: string;
  user?: string;
  // The `user@host:port` line as a screen shows it, so it can be typed back.
  destination?: string;
  path?: string;
  patterns?: readonly string[];
  tags?: readonly string[];
};

export function normalizeHostQuery(raw: string): string {
  return raw.trim().toLocaleLowerCase();
}

export function hostMatchesQuery(fields: HostSearchFields, normalizedQuery: string): boolean {
  if (normalizedQuery === "") return true;
  const candidates = [
    fields.alias, fields.group, fields.hostName, fields.user, fields.destination, fields.path,
    ...(fields.patterns ?? []), ...(fields.tags ?? []),
  ];
  return candidates.some((candidate) => candidate !== undefined && candidate.toLocaleLowerCase().includes(normalizedQuery));
}

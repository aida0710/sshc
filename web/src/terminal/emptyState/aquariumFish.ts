import type { ConnectionHistory } from "../../api/terminalOverview";
import type { AquariumFish } from "./aquariumArt";

// The days of activity that make a fish large: what is used these days, not what was used once a lot.
const weighedDays = 30;
const hourMilliseconds = 3_600_000;

// aquariumFish is a fish for each connection used lately, weighed by the hours it was opened in over the last
// weighedDays.
export function aquariumFish(history: ConnectionHistory | null, now: Date): AquariumFish[] {
  if (history === null) return [];
  const since = now.getTime() - weighedDays * 24 * hourMilliseconds;
  const hours = new Map<string, number>();
  for (const hour of history.hours) {
    if (Date.parse(hour.hour) < since) continue;
    for (const alias of hour.aliases) hours.set(alias, (hours.get(alias) ?? 0) + 1);
  }
  return history.recent.map((connection) => ({ label: connection.alias, weight: hours.get(connection.alias) ?? 0 }));
}

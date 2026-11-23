import type { ConnectionHour } from "../../api/terminalOverview";

// The connections of the engine's hours, put into the viewer's own days for the activity chart: a column a week
// from Sunday, the last column being this week up to today.

export type ActivityLevel = 0 | 1 | 2 | 3 | 4;

export type ActivityDay = {
  date: Date;
  // week is the column (0 is the oldest week) and weekday the row (0 is Sunday).
  week: number;
  weekday: number;
  connections: number;
  // level is how dark the day is drawn: 0 for none, then 1 to 4 against the busiest day shown.
  level: ActivityLevel;
};

export type ConnectionActivity = {
  days: ActivityDay[];
  firstDay: Date;
  connections: number;
  // hosts is how many different connections were used over the days shown.
  hosts: number;
};

const daysPerWeek = 7;
const busiestLevel = 4;
const millisecondsPerDay = 24 * 60 * 60 * 1000;

function startOfDay(moment: Date): Date {
  return new Date(moment.getFullYear(), moment.getMonth(), moment.getDate());
}

// daysBetween counts calendar days, so a day lengthened or shortened by a clock change still counts as one.
function daysBetween(from: Date, to: Date): number {
  return Math.round((startOfDay(to).getTime() - from.getTime()) / millisecondsPerDay);
}

export function connectionActivity(hours: readonly ConnectionHour[], today: Date, weeks: number): ConnectionActivity {
  const todayStart = startOfDay(today);
  const firstDay = new Date(todayStart.getFullYear(), todayStart.getMonth(), todayStart.getDate() - (weeks - 1) * daysPerWeek - todayStart.getDay());
  const shownDays = daysBetween(firstDay, todayStart) + 1;
  const counts = new Array<number>(shownDays).fill(0);
  const hosts = new Set<string>();
  let connections = 0;
  for (const hour of hours) {
    const index = daysBetween(firstDay, new Date(hour.hour));
    if (index < 0 || index >= shownDays) continue;
    counts[index] = (counts[index] ?? 0) + hour.connections;
    connections += hour.connections;
    for (const alias of hour.aliases) hosts.add(alias);
  }
  const busiest = Math.max(0, ...counts);
  const days = counts.map((count, index): ActivityDay => ({
    date: new Date(firstDay.getFullYear(), firstDay.getMonth(), firstDay.getDate() + index),
    week: Math.floor(index / daysPerWeek),
    weekday: index % daysPerWeek,
    connections: count,
    level: count === 0 ? 0 : (Math.max(1, Math.ceil((count / busiest) * busiestLevel)) as ActivityLevel),
  }));
  return { days, firstDay, connections, hosts: hosts.size };
}

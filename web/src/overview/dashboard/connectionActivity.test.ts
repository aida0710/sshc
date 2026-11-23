import { describe, expect, it } from "vitest";
import { connectionActivity } from "./connectionActivity";

// A Wednesday, so the last column holds Sunday to Wednesday.
const today = new Date(2026, 10, 11, 15, 0);

function hourAt(date: Date, connections: number, aliases: string[]) {
  return { hour: date.toISOString(), connections, aliases };
}

describe("connectionActivity", () => {
  it("lays the days out a week a column from Sunday, ending today", () => {
    const activity = connectionActivity([], today, 2);

    expect(activity.firstDay).toEqual(new Date(2026, 10, 1));
    expect(activity.days).toHaveLength(7 + 4);
    expect(activity.days.at(-1)).toMatchObject({ date: new Date(2026, 10, 11), week: 1, weekday: 3 });
  });

  it("counts an hour on the viewer's own day, not the engine's UTC day", () => {
    // 23:30 local time on the 10th, whatever the time zone of the test.
    const lateEvening = new Date(2026, 10, 10, 23, 30);
    const activity = connectionActivity([hourAt(lateEvening, 3, ["web-01"])], today, 1);

    expect(activity.days.find((day) => day.connections > 0)?.date).toEqual(new Date(2026, 10, 10));
  });

  it("adds up the connections and counts each host once over the days shown", () => {
    const activity = connectionActivity([
      hourAt(new Date(2026, 10, 9, 9), 2, ["web-01", "db"]),
      hourAt(new Date(2026, 10, 10, 9), 1, ["web-01"]),
      // Before the weeks shown: left out.
      hourAt(new Date(2026, 9, 1, 9), 5, ["old-host"]),
    ], today, 2);

    expect(activity.connections).toBe(3);
    expect(activity.hosts).toBe(2);
  });

  it("draws the busiest day darkest and any day with a connection at least lightly", () => {
    const activity = connectionActivity([
      hourAt(new Date(2026, 10, 9, 9), 40, ["web-01"]),
      hourAt(new Date(2026, 10, 10, 9), 1, ["web-01"]),
    ], today, 1);
    const levelOn = (date: number) => activity.days.find((day) => day.date.getDate() === date)?.level;

    expect(levelOn(9)).toBe(4);
    expect(levelOn(10)).toBe(1);
    expect(levelOn(11)).toBe(0);
  });
});

import { describe, expect, it } from "vitest";
import { noteNotificationVersions } from "./runTargetUnread";

describe("noteNotificationVersions", () => {
  it("treats the first look at a run target as history", () => {
    expect(noteNotificationVersions(undefined, [{ id: "a", notificationVersion: 3 }]).notified).toBe(false);
  });

  it("reports a session whose notification version moved since the last look", () => {
    const first = noteNotificationVersions(undefined, [{ id: "a", notificationVersion: 3 }, { id: "b" }]);
    expect(noteNotificationVersions(first.seen, [{ id: "a", notificationVersion: 3 }, { id: "b", notificationVersion: 1 }]).notified).toBe(true);
  });

  it("does not count a session that appeared since the last look", () => {
    const first = noteNotificationVersions(undefined, [{ id: "a", notificationVersion: 3 }]);
    expect(noteNotificationVersions(first.seen, [{ id: "a", notificationVersion: 3 }, { id: "new", notificationVersion: 2 }]).notified).toBe(false);
  });
});

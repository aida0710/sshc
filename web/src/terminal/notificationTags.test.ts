import { describe, expect, it } from "vitest";
import { longCommandNotificationTag, programNotificationTag, sessionIdFromNotificationTag } from "./notificationTags";

describe("terminal notification tags", () => {
  it("どちらの通知のtagからもセッションを取り出せる", () => {
    expect(sessionIdFromNotificationTag(programNotificationTag("abc123"))).toBe("abc123");
    expect(sessionIdFromNotificationTag(longCommandNotificationTag("abc123"))).toBe("abc123");
  });

  it("ターミナル以外の通知のtagからは取り出さない", () => {
    expect(sessionIdFromNotificationTag("sshc-notification-permission")).toBeNull();
    expect(sessionIdFromNotificationTag("sshc-transfer-1")).toBeNull();
    expect(sessionIdFromNotificationTag("sshc-terminal-")).toBeNull();
  });
});

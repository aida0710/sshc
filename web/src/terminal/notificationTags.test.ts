import { describe, expect, it } from "vitest";
import { sessionIdFromNotificationTag, terminalNotificationTag } from "./notificationTags";

describe("terminal notification tags", () => {
  it("ターミナルの通知のtagからセッションを取り出せる", () => {
    expect(sessionIdFromNotificationTag(terminalNotificationTag("abc123"))).toBe("abc123");
  });

  it("ターミナル以外の通知のtagからは取り出さない", () => {
    expect(sessionIdFromNotificationTag("sshc-notification-permission")).toBeNull();
    expect(sessionIdFromNotificationTag("sshc-transfer-1")).toBeNull();
    expect(sessionIdFromNotificationTag("sshc-terminal-")).toBeNull();
  });
});

import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/integrations";
import { en, ja, type MessageKey } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import {
  browserNotificationPermission,
  nextAgentNotification,
  requestBrowserNotificationPermission,
  showBrowserNotification,
} from "./agentNotifications";

const session: TerminalSession = {
  id: "one", kind: "ssh", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "agent", titlePinned: false },
  agent: {
    kind: "codex", state: "attention", resumable: true,
    observationVersion: 3, signalVersion: 2,
    lastSignal: { kind: "attention", occurredAt: "2026-08-29T01:01:00Z" },
  },
};

function translator(language: "en" | "ja"): Translate {
  const catalogue = language === "ja" ? ja : en;
  return ((key: MessageKey, values: Record<string, string> = {}) => {
    let message: string = catalogue[key];
    for (const [name, value] of Object.entries(values)) message = message.replaceAll(`{${name}}`, value);
    return message;
  }) as Translate;
}

describe("agent notification policy", () => {
  it("does not replay the latest signal on initial load", () => {
    expect(nextAgentNotification(translator("en"), session, undefined)).toBeNull();
  });

  it("emits only when signalVersion advances and keeps private fields out", () => {
    const notification = nextAgentNotification(translator("ja"), session, 1);
    expect(notification?.body).toBe("API認証の修正（osaka）が入力を待っています");
    expect(notification?.body).not.toContain("thread");
    expect(nextAgentNotification(translator("ja"), session, 2)).toBeNull();
  });
});

describe("browser notification permission", () => {
  it("reports browsers without a usable Notification API as unsupported", async () => {
    const target = {} as Window;
    expect(browserNotificationPermission(target)).toBe("unsupported");
    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("unsupported");
  });

  it("requests permission only while it is undecided", async () => {
    class FakeNotification {
      static permission: NotificationPermission = "default";
      static requestPermission = vi.fn(async () => {
        FakeNotification.permission = "granted";
        return "granted" as const;
      });
    }
    const target = { Notification: FakeNotification } as unknown as Window;

    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("granted");
    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("granted");
    expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1);
  });

  it("delivers only after permission is granted", () => {
    const delivered: Array<{ title: string; options: NotificationOptions | undefined }> = [];
    class FakeNotification {
      static permission: NotificationPermission = "denied";
      static requestPermission = vi.fn();

      constructor(title: string, options?: NotificationOptions) {
        delivered.push({ title, options });
      }
    }
    const target = { Notification: FakeNotification } as unknown as Window;
    const message = { title: "sshc", body: "ready", tag: "test" };

    expect(showBrowserNotification(message, target)).toBe(false);
    FakeNotification.permission = "granted";
    expect(showBrowserNotification(message, target)).toBe(true);
    expect(delivered).toEqual([{ title: "sshc", options: { body: "ready", tag: "test" } }]);
  });
});

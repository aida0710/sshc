import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/integrations";
import { en, ja, type MessageKey } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import { nextAgentNotification } from "./agentNotifications";

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

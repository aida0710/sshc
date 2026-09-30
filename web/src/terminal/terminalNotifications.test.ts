import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { en, ja, type MessageKey } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import { nextTerminalNotification } from "./terminalNotifications";

const session: TerminalSession = {
  id: "one", kind: "ssh", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "terminal", titlePinned: false },
  notificationVersion: 2,
  lastNotification: { title: "Claude Code", body: "入力を待っています", occurredAt: "2026-08-29T01:01:00Z" },
};

function translator(language: "en" | "ja"): Translate {
  const catalogue = language === "ja" ? ja : en;
  return ((key: MessageKey, values: Record<string, string> = {}) => {
    let message: string = catalogue[key];
    for (const [name, value] of Object.entries(values)) message = message.replaceAll(`{${name}}`, value);
    return message;
  }) as Translate;
}

describe("terminal notification policy", () => {
  it("does not replay the latest notification on initial load", () => {
    expect(nextTerminalNotification(translator("en"), session, undefined)).toBeNull();
  });

  it("emits only when notificationVersion advances and names the pane", () => {
    const notification = nextTerminalNotification(translator("ja"), session, 1);
    expect(notification).toEqual({ title: "API認証の修正（osaka）", body: "Claude Code\n入力を待っています" });
    expect(nextTerminalNotification(translator("ja"), session, 2)).toBeNull();
  });

  it("falls back to a generic body when the program sent no text", () => {
    const bare: TerminalSession = { ...session, lastNotification: { title: "", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translator("en"), bare, 1)?.body).toBe(en["terminal.notificationFallback"]);
    const titleOnly: TerminalSession = { ...session, lastNotification: { title: "Build finished", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translator("en"), titleOnly, 1)?.body).toBe("Build finished");
  });
});

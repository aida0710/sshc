import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { en } from "../i18n/messages";
import { translatorFor } from "../testing/translator";
import { nextTerminalNotification } from "./terminalNotifications";

const session: TerminalSession = {
  id: "one", kind: "ssh", workspaceId: "default", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "terminal", titlePinned: false },
  notificationVersion: 2,
  lastNotification: { title: "Claude Code", body: "入力を待っています", occurredAt: "2026-08-29T01:01:00Z" },
};

describe("terminal notification policy", () => {
  it("does not replay the latest notification on initial load", () => {
    expect(nextTerminalNotification(translatorFor("en"), session, undefined)).toBeNull();
  });

  it("emits only when notificationVersion advances and names the pane", () => {
    const notification = nextTerminalNotification(translatorFor("ja"), session, 1);
    expect(notification).toEqual({ title: "osaka（API認証の修正）", body: "Claude Code\n入力を待っています" });
    expect(nextTerminalNotification(translatorFor("ja"), session, 2)).toBeNull();
  });

  it("leads the heading with the connection alias, which the server cannot set", () => {
    const disguised: TerminalSession = {
      ...session,
      presentation: { displayTitle: "sshc", titleSource: "terminal", titlePinned: false },
      lastNotification: { title: "Vault", body: "Unlock the vault again", occurredAt: "2026-08-29T01:01:00Z" },
    };
    expect(nextTerminalNotification(translatorFor("en"), disguised, 1)?.title).toBe("osaka（sshc）");
  });

  it("falls back to a generic body when the program sent no text", () => {
    const bare: TerminalSession = { ...session, lastNotification: { title: "", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translatorFor("en"), bare, 1)?.body).toBe(en["terminal.notificationFallback"]);
    const titleOnly: TerminalSession = { ...session, lastNotification: { title: "Build finished", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translatorFor("en"), titleOnly, 1)?.body).toBe("Build finished");
  });

  it("says how long a finished command ran and whether it failed", () => {
    const finished = (finishedCommand: { seconds: number; exitCode?: number }): TerminalSession => ({
      ...session,
      lastNotification: { title: "", body: "", occurredAt: "2026-10-28T03:00:31Z", finishedCommand },
    });
    expect(nextTerminalNotification(translatorFor("ja"), finished({ seconds: 31, exitCode: 0 }), 1))
      .toEqual({ title: "osaka（API認証の修正）", body: "コマンドが31秒で完了しました。" });
    expect(nextTerminalNotification(translatorFor("en"), finished({ seconds: 95, exitCode: 2 }), 1)?.body)
      .toBe("A command failed with exit code 2 after 95 seconds.");
    // A shell that reports no exit code is not taken to have failed.
    expect(nextTerminalNotification(translatorFor("en"), finished({ seconds: 40 }), 1)?.body)
      .toBe("A command finished after 40 seconds.");
  });
});

import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { terminalNoticeFor } from "./terminalNotice";

const live = { phase: "live" } as const;
const session: TerminalSession = {
  id: "s1", kind: "ssh", alias: "web-01", workspaceId: "default", title: "web-01", startedAt: "2026-11-02T09:00:00Z", state: "connected", problem: "",
};
const ended = (exited: NonNullable<TerminalSession["exited"]>, problem = ""): TerminalSession => ({ ...session, state: "exited", problem, exited });

describe("terminalNoticeFor", () => {
  it("shows no card while the session works", () => {
    expect(terminalNoticeFor(session, live)).toBeNull();
  });

  it("leaves a clean exit to the strip, and puts a lost connection or a failed program in a card", () => {
    expect(terminalNoticeFor(ended({ code: 0, signal: "", at: "2026-11-02T09:30:00Z" }), live)).toBeNull();
    expect(terminalNoticeFor(ended({ code: -1, signal: "", at: "2026-11-02T09:30:00Z", connectionLost: true }), live)?.kind).toBe("ended");
    expect(terminalNoticeFor(ended({ code: 3, signal: "", at: "2026-11-02T09:30:00Z" }), live)?.kind).toBe("ended");
    expect(terminalNoticeFor(ended({ code: 0, signal: "", at: "2026-11-02T09:30:00Z" }, "reconnect_stopped"), live)?.kind).toBe("ended");
  });

  it("names each ending apart, so a card hidden for one ending shows for the next", () => {
    const first = terminalNoticeFor(ended({ code: 3, signal: "", at: "2026-11-02T09:30:00Z" }), live);
    const second = terminalNoticeFor(ended({ code: 3, signal: "", at: "2026-11-02T10:30:00Z" }), live);
    expect(first?.key).not.toBe(second?.key);
  });

  it("keeps one card through every attempt of an automatic reconnect", () => {
    const attempt = (number: number): TerminalSession => ({
      ...session, state: "reconnecting", reconnect: { attempt: number, limit: 5, retryAt: "2026-11-02T09:31:00Z", problem: "reconnect_failed" },
    });
    expect(terminalNoticeFor(attempt(1), live)).toEqual(terminalNoticeFor(attempt(2), live));
    expect(terminalNoticeFor(attempt(1), live)?.kind).toBe("reconnecting");
  });

  it("shows a card for the link to the engine only once it stopped retrying, and not for a session that is gone", () => {
    expect(terminalNoticeFor(session, { phase: "waiting", attempt: 2, seconds: 3 })).toBeNull();
    expect(terminalNoticeFor(session, { phase: "stopped", gone: false })?.kind).toBe("link-stopped");
    expect(terminalNoticeFor(session, { phase: "stopped", gone: true })).toBeNull();
  });
});

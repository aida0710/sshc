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
    const reconnectedByHand = { ...ended({ code: 3, signal: "", at: "2026-11-02T10:30:00Z" }), startedAt: "2026-11-02T10:00:00Z" };
    expect(first?.key).not.toBe(terminalNoticeFor(reconnectedByHand, live)?.key);
  });

  it("keeps one card while the list catches up with the stream that reported the end", () => {
    const reported = terminalNoticeFor(ended({ code: 3, signal: "", at: "" }), live);
    const listed = terminalNoticeFor(ended({ code: 3, signal: "", at: "2026-11-02T09:30:00Z" }), live);
    expect(reported?.key).toBe(listed?.key);
  });

  it("keeps one card through every attempt of an automatic reconnect", () => {
    const attempt = (number: number): TerminalSession => ({
      ...session, state: "reconnecting", reconnect: { attempt: number, limit: 5, retryAt: "2026-11-02T09:31:00Z", problem: "reconnect_failed" },
    });
    expect(terminalNoticeFor(attempt(1), live)).toEqual(terminalNoticeFor(attempt(2), live));
    expect(terminalNoticeFor(attempt(1), live)?.kind).toBe("reconnecting");
  });

  it("asks about an unknown host key while connecting, once per key", () => {
    const asking = (fingerprint: string): TerminalSession => ({
      ...session, state: "connecting",
      hostKeyQuestion: { alias: "bastion", hostName: "192.0.2.10", keyType: "ssh-ed25519", fingerprint },
    });
    expect(terminalNoticeFor(asking("SHA256:first"), live)?.kind).toBe("host-key");
    // With ProxyJump the next hop may ask about its own key: a card hidden for the first one shows for it.
    expect(terminalNoticeFor(asking("SHA256:first"), live)?.key).not.toBe(terminalNoticeFor(asking("SHA256:second"), live)?.key);
  });

  it("asks for the notification permission only when nothing about the session comes first", () => {
    expect(terminalNoticeFor(session, live, true)?.kind).toBe("notification-permission");
    const lost = ended({ code: -1, signal: "", at: "2026-11-02T09:30:00Z", connectionLost: true });
    expect(terminalNoticeFor(lost, live, true)?.kind).toBe("ended");
  });

  it("asks about a typed password in a session that works, after what stops it working", () => {
    const asking: TerminalSession = { ...session, passwordSaveQuestion: true };
    expect(terminalNoticeFor(asking, live)?.kind).toBe("password-save");
    expect(terminalNoticeFor(asking, { phase: "stopped", gone: false })?.kind).toBe("link-stopped");
    expect(terminalNoticeFor({ ...asking, state: "reconnecting" }, live)?.kind).toBe("reconnecting");
    expect(terminalNoticeFor(asking, live, true)?.kind).toBe("password-save");
  });

  it("shows a card for the link to the engine only once it stopped retrying, and not for a session that is gone", () => {
    expect(terminalNoticeFor(session, { phase: "waiting", attempt: 2, seconds: 3 })).toBeNull();
    expect(terminalNoticeFor(session, { phase: "stopped", gone: false })?.kind).toBe("link-stopped");
    expect(terminalNoticeFor(session, { phase: "stopped", gone: true })).toBeNull();
  });
});

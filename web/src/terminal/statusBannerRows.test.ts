import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { noticeSeverity, statusBannerRows } from "./statusBannerRows";

const live = { phase: "live" } as const;
const session: TerminalSession = {
  id: "s1", kind: "ssh", alias: "web-01", workspaceId: "default", title: "web-01", startedAt: "2026-11-02T09:00:00Z", state: "connected", problem: "",
};
const quiet = { problem: "", link: live, reconnectFailed: false, answersHostKey: true };

describe("statusBannerRows", () => {
  it("has nothing to say for a session that works", () => {
    const rows = statusBannerRows({ ...quiet, session });
    expect(rows).toEqual([]);
    expect(noticeSeverity(rows, { session, link: live })).toBeNull();
  });

  it("says an ended session's reason once, in its ended row", () => {
    const ended: TerminalSession = { ...session, state: "exited", problem: "reconnect_stopped", exited: { code: -1, signal: "", at: "2026-11-02T09:30:00Z" } };
    expect(statusBannerRows({ ...quiet, session: ended })).toEqual(["ended"]);
  });

  it("asks about a host key only where a button can answer it", () => {
    const asking: TerminalSession = {
      ...session, state: "connecting",
      hostKeyQuestion: { alias: "web-01", hostName: "10.0.2.15", keyType: "ssh-ed25519", fingerprint: "SHA256:x" },
    };
    expect(statusBannerRows({ ...quiet, session: asking })).toEqual(["host-key"]);
    expect(statusBannerRows({ ...quiet, session: asking, answersHostKey: false })).toEqual([]);
  });

  it("is red once something ended or failed, and amber while the engine works on it or a question waits", () => {
    const reconnecting: TerminalSession = { ...session, state: "reconnecting" };
    const rows = (shown: TerminalSession, link: typeof quiet.link | { phase: "stopped"; gone: boolean } = live) =>
      noticeSeverity(statusBannerRows({ ...quiet, session: shown, link }), { session: shown, link });
    expect(rows(reconnecting)).toBe("notice");
    expect(rows(session, { phase: "stopped", gone: false })).toBe("danger");
    expect(rows({ ...session, state: "exited", exited: { code: 0, signal: "", at: "2026-11-02T09:30:00Z" } })).toBe("notice");
    expect(rows({ ...session, state: "exited", exited: { code: 3, signal: "", at: "2026-11-02T09:30:00Z" } })).toBe("danger");
  });
});

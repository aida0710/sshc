import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { translatorFor } from "../testing/translator";
import { exitedAbnormally, sessionMarkerClass, sessionStatusText, sessionStatusTone } from "./sessionStatus";


const base: TerminalSession = {
  id: "a", kind: "ssh", alias: "bastion", workspaceId: "default", title: "bastion", startedAt: "2026-10-28T09:00:00Z", state: "exited", problem: "",
};
const ended = (exited: NonNullable<TerminalSession["exited"]>): TerminalSession => ({ ...base, exited });

describe("a session that has ended", () => {
  it("counts a non-zero status, a signal and a lost connection as failures", () => {
    expect(exitedAbnormally(ended({ code: 0, signal: "", at: "" }))).toBe(false);
    expect(exitedAbnormally(ended({ code: 3, signal: "", at: "" }))).toBe(true);
    expect(exitedAbnormally(ended({ code: -1, signal: "SIGKILL", at: "" }))).toBe(true);
    expect(exitedAbnormally(ended({ code: -1, signal: "", at: "", connectionLost: true }))).toBe(true);
    expect(exitedAbnormally({ ...base, state: "connected" })).toBe(false);
  });

  it("counts a session that ended as soon as it opened as a failure, even with status 0", () => {
    const rightAway: TerminalSession = { ...ended({ code: 0, signal: "", at: "2026-10-28T09:00:00.120Z" }) };
    const ranAWhile: TerminalSession = { ...ended({ code: 0, signal: "", at: "2026-10-28T09:00:01Z" }) };
    const t = translatorFor("en");

    expect(exitedAbnormally(rightAway)).toBe(true);
    expect(sessionStatusText(t, rightAway)).toBe("exited right after opening");
    expect(exitedAbnormally(ranAWhile)).toBe(false);
    expect(sessionStatusText(t, ranAWhile)).toBe("exited 0");
    // The stream reports the exit before the list says when it happened.
    expect(exitedAbnormally(ended({ code: 0, signal: "", at: "" }))).toBe(false);
  });

  it("marks a failure in red and a clean exit faintly", () => {
    const failed = ended({ code: 3, signal: "", at: "" });
    const clean = ended({ code: 0, signal: "", at: "" });

    expect(sessionMarkerClass(failed, false)).toBe("bg-danger");
    expect(sessionStatusTone(failed)).toBe("text-danger");
    expect(sessionMarkerClass(clean, false)).toBe("bg-ink-faint");
    expect(sessionStatusTone(clean)).toBe("text-ink-faint");
  });

  it("says the connection was lost instead of exit -1, and names a signal", () => {
    const t = translatorFor("en");
    expect(sessionStatusText(t, ended({ code: -1, signal: "", at: "", connectionLost: true }))).toBe("connection lost");
    expect(sessionStatusText(t, ended({ code: -1, signal: "SIGTERM", at: "" }))).toBe("ended by SIGTERM");
    expect(sessionStatusText(t, ended({ code: 3, signal: "", at: "" }))).toBe("exited 3");
  });
});

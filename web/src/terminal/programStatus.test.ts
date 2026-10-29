import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { translatorFor } from "../testing/translator";
import { programStatusText, rowProgramStatus, type ProgramStatus } from "./programStatus";

const t = translatorFor("en");

function sessionWith(programStatus: ProgramStatus | undefined, overrides: Partial<TerminalSession> = {}): TerminalSession {
  return {
    id: "s", kind: "shell", workspaceId: "default", title: "claude", startedAt: "2026-10-29T09:00:00Z", state: "connected", problem: "",
    ...(programStatus === undefined ? {} : { programStatus }),
    ...overrides,
  };
}

describe("rowProgramStatus", () => {
  it("shows working and blocked for as long as they last, even once seen", () => {
    for (const state of ["working", "blocked"] as const) {
      expect(rowProgramStatus(sessionWith({ state, version: 3 }), new Map([["s", 3]]))?.state).toBe(state);
    }
  });

  it("shows a done or failed result only until the session has been on screen since it came", () => {
    for (const state of ["done", "error"] as const) {
      const session = sessionWith({ state, version: 4 });
      expect(rowProgramStatus(session, new Map())?.state).toBe(state);
      expect(rowProgramStatus(session, new Map([["s", 3]]))?.state).toBe(state);
      expect(rowProgramStatus(session, new Map([["s", 4]]))).toBeNull();
    }
  });

  it("shows nothing for an idle program", () => {
    expect(rowProgramStatus(sessionWith({ state: "idle", version: 1 }), new Map())).toBeNull();
  });

  it("leaves the row to the connection while it reconnects, has a problem or has ended", () => {
    const blocked: ProgramStatus = { state: "blocked", version: 1 };
    expect(rowProgramStatus(sessionWith(blocked, { state: "reconnecting" }), new Map())).toBeNull();
    expect(rowProgramStatus(sessionWith(blocked, { problem: "transport_lost" }), new Map())).toBeNull();
    expect(rowProgramStatus(sessionWith(blocked, { state: "exited", exited: { code: 0, signal: "", at: "2026-10-29T09:01:00Z" } }), new Map())).toBeNull();
  });
});

describe("programStatusText", () => {
  it("says what is awaited, then what the program said", () => {
    expect(programStatusText(t, { state: "blocked", kind: "permission", message: "Run rm -rf build?", version: 1 })).toBe("Needs permission · Run rm -rf build?");
    expect(programStatusText(t, { state: "blocked", version: 1 })).toBe("Waiting for you");
  });

  it("gives the progress while working, and the title when there is no message", () => {
    expect(programStatusText(t, { state: "working", progress: 40, title: "docs", version: 1 })).toBe("Working 40% · docs");
    expect(programStatusText(t, { state: "error", message: "rsync failed", title: "deploy", version: 1 })).toBe("Failed · rsync failed");
  });
});

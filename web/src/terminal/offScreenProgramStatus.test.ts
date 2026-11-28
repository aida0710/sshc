import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { offScreenProgramStatus } from "./offScreenProgramStatus";
import type { ProgramStatus } from "./programStatus";

function session(id: string, programStatus?: ProgramStatus): TerminalSession {
  return {
    id, kind: "ssh", alias: id, workspaceId: "default", title: id, startedAt: "2026-11-28T09:00:00Z", state: "connected", problem: "",
    ...(programStatus === undefined ? {} : { programStatus }),
  };
}

const mirageWaits = session("mirage", { state: "blocked", kind: "permission", app: "claude", version: 3 });
const pelicanWorks = session("pelican", { state: "working", app: "claude", version: 2 });
const r100Done = session("r100", { state: "done", app: "claude", version: 5 });
const r100Failed = session("r100-lint", { state: "error", app: "claude", version: 4 });
const dunlin = session("dunlin");

describe("offScreenProgramStatus", () => {
  it("says a program waits in another session before saying one failed or finished", () => {
    const sessions = [r100Done, r100Failed, mirageWaits, pelicanWorks, dunlin];
    expect(offScreenProgramStatus({ sessions, seen: new Map(), sessionsOnScreen: ["dunlin"] })).toBe("blocked");
  });

  it("says a program failed before saying one finished", () => {
    const sessions = [r100Done, r100Failed, pelicanWorks, dunlin];
    expect(offScreenProgramStatus({ sessions, seen: new Map(), sessionsOnScreen: ["dunlin"] })).toBe("error");
  });

  it("says a program finished when nothing else needs the person", () => {
    expect(offScreenProgramStatus({ sessions: [r100Done, pelicanWorks, dunlin], seen: new Map(), sessionsOnScreen: ["dunlin"] })).toBe("done");
  });

  it("leaves out the sessions on the screen, whose program the person can see", () => {
    const sessions = [mirageWaits, r100Done, dunlin];
    expect(offScreenProgramStatus({ sessions, seen: new Map(), sessionsOnScreen: ["mirage", "r100"] })).toBeNull();
  });

  it("counts every session while no terminal is on the screen", () => {
    expect(offScreenProgramStatus({ sessions: [mirageWaits, dunlin], seen: new Map(), sessionsOnScreen: [] })).toBe("blocked");
  });

  it("leaves out a result the person has had on the screen since it came", () => {
    expect(offScreenProgramStatus({ sessions: [r100Done, dunlin], seen: new Map([["r100", 5]]), sessionsOnScreen: ["dunlin"] })).toBeNull();
  });

  it("says nothing while the programs only work", () => {
    expect(offScreenProgramStatus({ sessions: [pelicanWorks, dunlin], seen: new Map(), sessionsOnScreen: ["dunlin"] })).toBeNull();
  });
});

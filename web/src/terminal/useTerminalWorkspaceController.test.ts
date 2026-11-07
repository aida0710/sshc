import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import type { TerminalSessionsState } from "./sessions";
import { paneSessionIds } from "./splitView/layout";
import { useTerminalWorkspaceController } from "./useTerminalWorkspaceController";
import type { Section } from "../routing/sectionRoute";

function session(id: string, workspaceId = "default"): TerminalSession {
  return {
    id,
    kind: "ssh", workspaceId,
    alias: id,
    title: id,
    startedAt: "2026-01-01T00:00:00Z",
    state: "connected",
    problem: "",
  };
}

const workspaces: TerminalWorkspace[] = [
  { id: "default", name: "", hidden: false, builtIn: true },
  { id: "agents", name: "", hidden: true, builtIn: true },
];

function useController(list: TerminalSession[], refresh: () => Promise<void>, section: Section = "Terminal") {
  return useTerminalWorkspaceController({
    terminalSessions: {
      sessions: list,
      maxSessions: 50,
      busy: false,
      problem: "",
      loaded: true,
      refresh,
    } as unknown as TerminalSessionsState,
    workspaces,
    section,
    navigate: () => undefined,
    closeNavigation: () => undefined,
  });
}

describe("useTerminalWorkspaceController", () => {
  it("opens a session in a new pane beside the one shown, and keeps the one shown on the screen", async () => {
    const { result, rerender } = renderHook(({ listed }: { listed: TerminalSession[] }) => useTerminalWorkspaceController({
        terminalSessions: {
        sessions: listed,
        maxSessions: 50,
        refresh: vi.fn().mockResolvedValue(undefined),
        open: vi.fn(async () => session("opened")),
      } as unknown as TerminalSessionsState,
      workspaces,
      section: "Terminal",
      navigate: () => undefined,
      closeNavigation: () => undefined,
    }), { initialProps: { listed: [session("first")] } });
    await waitFor(() => expect(result.current.activeSessionId).toBe("first"));

    // The list holds the new session by the time the split lands, as terminalSessions.open sees to.
    rerender({ listed: [session("first"), session("opened")] });
    await act(async () => {
      await result.current.openSessionInSplit({ kind: "shell" }, { targetSessionId: "first", edge: "right" });
    });

    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["first", "opened"]);
    expect(result.current.activeSessionId).toBe("opened");
  });

  it("opens a local folder with the requested shell profile and keeps failures on the current screen", async () => {
    const local: TerminalSession = { ...session("local"), kind: "shell" };
    delete local.alias;
    const opened: (TerminalSession | null)[] = [local, null];
    const open = vi.fn(async () => opened.shift() ?? null);
    const navigate = vi.fn();
    const { result } = renderHook(() => useTerminalWorkspaceController({
        terminalSessions: { sessions: [local], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      workspaces,
      section: "Files",
      navigate,
      closeNavigation: () => undefined,
    }));

    await act(() => result.current.openLocalShell("fish", "/home/engine/project's work"));
    expect(open).toHaveBeenLastCalledWith({ kind: "shell", profileId: "fish", cwd: "/home/engine/project's work" });
    expect(navigate).toHaveBeenCalledWith("Terminal");
    navigate.mockClear();
    await act(() => result.current.openLocalShell(undefined, "/missing"));
    expect(open).toHaveBeenLastCalledWith({ kind: "shell", cwd: "/missing" });
    expect(navigate).not.toHaveBeenCalled();
  });

  it("keeps a freshly opened session selected while the session list catches up", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const listed = [session("s1"), session("s2")];
    const { result, rerender } = renderHook(
      ({ list }: { list: TerminalSession[] }) => useController(list, refresh),
      { initialProps: { list: listed } },
    );
    await waitFor(() => expect(result.current.activeSessionId).toBe("s1"));

    // Quick Connect opens the session through the API, so the list still lacks it.
    act(() => result.current.showSession("s3"));
    expect(result.current.activeSessionId).toBe("s3");
    expect(refresh).toHaveBeenCalled();

    await act(async () => {
      rerender({ list: [...listed, session("s3")] });
    });
    expect(result.current.activeSessionId).toBe("s3");
  });

  it("falls back to the top of the sidebar once the shown session is gone", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const listed = [session("s1"), session("s2")];
    const { result, rerender } = renderHook(
      ({ list }: { list: TerminalSession[] }) => useController(list, refresh),
      { initialProps: { list: listed } },
    );
    await waitFor(() => expect(result.current.activeSessionId).toBe("s1"));

    await act(async () => {
      result.current.showSession("s2");
    });
    expect(result.current.activeSessionId).toBe("s2");

    await act(async () => {
      rerender({ list: [session("s1")] });
    });
    expect(result.current.activeSessionId).toBe("s1");
  });

  it("shows an opened SSH session in the terminal section, and stays put when it could not be opened", async () => {
    const opened: (TerminalSession | null)[] = [session("s1"), null];
    const open = vi.fn(async () => opened.shift() ?? null);
    const navigate = vi.fn();
    const { result } = renderHook(() =>
      useTerminalWorkspaceController({
            terminalSessions: {
          sessions: [session("s0"), session("s1")],
          refresh: vi.fn().mockResolvedValue(undefined),
          open,
        } as unknown as TerminalSessionsState,
        workspaces,
        section: "Files",
        navigate,
        closeNavigation: () => undefined,
      }),
    );
    await waitFor(() => expect(result.current.activeSessionId).toBe("s0"));

    await act(() => result.current.openSSHSession("s1", "/srv/app"));
    expect(open).toHaveBeenLastCalledWith({ kind: "ssh", alias: "s1", cwd: "/srv/app" });
    expect(result.current.activeSessionId).toBe("s1");
    expect(navigate).toHaveBeenCalledWith("Terminal");

    navigate.mockClear();
    await act(() => result.current.openSSHSession("s2"));
    expect(open).toHaveBeenLastCalledWith({ kind: "ssh", alias: "s2" });
    expect(result.current.activeSessionId).toBe("s1");
    expect(navigate).not.toHaveBeenCalled();
  });
  it("opens a duplicate in the directory and shell profile of the original", async () => {
    const remote: TerminalSession = { ...session("edge"), workingDirectory: "~/src/sshc", workspaceId: "work" };
    const local: TerminalSession = { ...session("local"), kind: "shell", shellProfileId: "fish", workingDirectory: "/home/aida/notes" };
    delete local.alias;
    const unknown: TerminalSession = session("plain");
    const open = vi.fn(async () => null);
    const { result } = renderHook(() => useTerminalWorkspaceController({
        terminalSessions: { sessions: [remote, local, unknown], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      workspaces,
      section: "Terminal",
      navigate: () => undefined,
      closeNavigation: () => undefined,
    }));

    await act(() => result.current.duplicateSession("edge"));
    expect(open).toHaveBeenLastCalledWith({ kind: "ssh", alias: "edge", workspaceId: "work", cwd: "~/src/sshc" });
    await act(() => result.current.duplicateSession("local"));
    expect(open).toHaveBeenLastCalledWith({ kind: "shell", workspaceId: "default", cwd: "/home/aida/notes", profileId: "fish" });
    await act(() => result.current.duplicateSession("plain"));
    expect(open).toHaveBeenLastCalledWith({ kind: "ssh", alias: "plain", workspaceId: "default" });
  });
  it("shows the terminal screen at once when a session opens on a run target", async () => {
    let finish: (session: TerminalSession | null) => void = () => undefined;
    const open = vi.fn(() => new Promise<TerminalSession | null>((resolve) => { finish = resolve; }));
    const navigate = vi.fn();
    const { result } = renderHook(() => useTerminalWorkspaceController({
        runTarget: "r100",
      terminalSessions: { sessions: [], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      workspaces,
      section: "Home",
      navigate,
      closeNavigation: () => undefined,
    }));

    let opening: Promise<void> = Promise.resolve();
    act(() => { opening = result.current.openSSHSession("db"); });
    expect(navigate).toHaveBeenCalledWith("Terminal");
    await act(async () => { finish(null); await opening; });
  });

  it("opens a session for each chosen connection in order and shows the first as soon as it opens", async () => {
    const openEach = vi.fn(async (_requests: unknown, opened: (session: TerminalSession) => void) => {
      opened(session("web"));
      opened(session("db"));
      return [];
    });
    const navigate = vi.fn();
    const { result } = renderHook(() => useTerminalWorkspaceController({
      terminalSessions: { sessions: [session("web"), session("db")], refresh: vi.fn().mockResolvedValue(undefined), openEach } as unknown as TerminalSessionsState,
      workspaces,
      section: "Connections",
      navigate,
      closeNavigation: () => undefined,
    }));

    await act(() => result.current.openSSHSessions(["web", "db"]));
    expect(openEach).toHaveBeenCalledWith([{ kind: "ssh", alias: "web" }, { kind: "ssh", alias: "db" }], expect.any(Function));
    expect(result.current.activeSessionId).toBe("web");
    expect(navigate).toHaveBeenCalledWith("Terminal");
  });

  it("opens every connection a locked remote Vault held back once it is unlocked", async () => {
    const openEach = vi.fn()
      .mockImplementationOnce(async (requests: unknown[]) => requests)
      .mockImplementationOnce(async () => []);
    const terminalSessions = { sessions: [], refresh: vi.fn().mockResolvedValue(undefined), openEach, remoteRefusal: null } as unknown as TerminalSessionsState;
    const { result, rerender } = renderHook(({ vaultRevision }: { vaultRevision: number }) => useTerminalWorkspaceController({
      runTarget: "r100",
      vaultRevision,
      terminalSessions,
      workspaces,
      section: "Connections",
      navigate: () => undefined,
      closeNavigation: () => undefined,
    }), { initialProps: { vaultRevision: 0 } });

    await act(() => result.current.openSSHSessions(["web", "db"]));
    expect(openEach).toHaveBeenCalledOnce();
    await act(async () => rerender({ vaultRevision: 1 }));
    expect(openEach).toHaveBeenLastCalledWith([{ kind: "ssh", alias: "web" }, { kind: "ssh", alias: "db" }], expect.any(Function));
  });

  it("shows again the session last shown on each run target when coming back to it", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const here = [session("s1"), session("s2")];
    const bastion = [session("b1"), session("b2")];
    const { result, rerender } = renderHook(
      ({ list, runTarget }: { list: TerminalSession[]; runTarget: string | null }) => useTerminalWorkspaceController({ workspaces, section: "Terminal", navigate: () => undefined, closeNavigation: () => undefined, runTarget,
        terminalSessions: { sessions: list, refresh } as unknown as TerminalSessionsState,
      }),
      { initialProps: { list: here, runTarget: null as string | null } },
    );
    await waitFor(() => expect(result.current.activeSessionId).toBe("s1"));
    await act(async () => result.current.showSession("s2"));

    // The switching render has no list yet: the next engine has not answered.
    await act(async () => rerender({ list: [], runTarget: "bastion" }));
    expect(result.current.activeSessionId).toBeNull();
    await act(async () => rerender({ list: bastion, runTarget: "bastion" }));
    expect(result.current.activeSessionId).toBe("b1");
    await act(async () => result.current.showSession("b2"));

    await act(async () => rerender({ list: [], runTarget: null }));
    await act(async () => rerender({ list: here, runTarget: null }));
    expect(result.current.activeSessionId).toBe("s2");
    await act(async () => rerender({ list: [], runTarget: "bastion" }));
    await act(async () => rerender({ list: bastion, runTarget: "bastion" }));
    expect(result.current.activeSessionId).toBe("b2");

    // A session closed on bastion while away is not waited for: the top of the sidebar is shown.
    await act(async () => rerender({ list: [], runTarget: null }));
    await act(async () => rerender({ list: here, runTarget: null }));
    await act(async () => rerender({ list: [], runTarget: "bastion" }));
    await act(async () => rerender({ list: [session("b1")], runTarget: "bastion" }));
    expect(result.current.activeSessionId).toBe("b1");
  });

  it("passes over the sessions of hidden workspaces for the top of the sidebar", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useController([session("agent-job", "agents"), session("main")], refresh));
    await waitFor(() => expect(result.current.activeSessionId).toBe("main"));
  });

  it("chooses no session before the workspaces are read, for the sidebar has no top yet", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const listed = [session("s1")];
    const { result, rerender } = renderHook(
      ({ read }: { read: TerminalWorkspace[] }) => useTerminalWorkspaceController({ workspaces: read, section: "Terminal", navigate: () => undefined, closeNavigation: () => undefined,
        terminalSessions: { sessions: listed, refresh } as unknown as TerminalSessionsState,
      }),
      { initialProps: { read: [] as TerminalWorkspace[] } },
    );
    await act(async () => undefined);
    expect(result.current.activeSessionId).toBeNull();
    await act(async () => rerender({ read: workspaces }));
    expect(result.current.activeSessionId).toBe("s1");
  });
});

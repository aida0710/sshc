import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalSessionsState } from "./sessions";
import { useTerminalWorkspaceController } from "./useTerminalWorkspaceController";
import type { SettingsApi } from "../api/settings";
import type { Section } from "../routing/sectionRoute";

function session(id: string): TerminalSession {
  return {
    id,
    kind: "ssh", workspaceId: "default",
    alias: id,
    title: id,
    startedAt: "2026-01-01T00:00:00Z",
    state: "connected",
    problem: "",
  };
}

const api = {
  terminalSettings: vi.fn().mockResolvedValue({}),
} as unknown as SettingsApi;
const shells = { shellProfiles: vi.fn().mockResolvedValue({ profiles: [] }) };

function useController(list: TerminalSession[], refresh: () => Promise<void>, section: Section = "Terminal") {
  return useTerminalWorkspaceController({
    api,
    shells,
    terminalSessions: {
      sessions: list,
      maxSessions: 50,
      busy: false,
      problem: "",
      loaded: true,
      refresh,
    } as unknown as TerminalSessionsState,
    enabled: true,
    section,
    navigate: () => undefined,
    closeNavigation: () => undefined,
  });
}

describe("useTerminalWorkspaceController", () => {
  it("offers the shells of the engine in use and reads them again when the run target changes", async () => {
    const shell = (id: string) => ({ id, label: id, path: `/bin/${id}`, arguments: [], default: true });
    const here = { shellProfiles: vi.fn().mockResolvedValue({ profiles: [shell("zsh")] }) };
    const bastion = { shellProfiles: vi.fn().mockResolvedValue({ profiles: [shell("bash")] }) };
    const { result, rerender } = renderHook(({ engine }: { engine: typeof here }) => useTerminalWorkspaceController({
      api,
      shells: engine,
      terminalSessions: { sessions: [], refresh: vi.fn().mockResolvedValue(undefined) } as unknown as TerminalSessionsState,
      enabled: true,
      section: "Terminal",
      navigate: () => undefined,
      closeNavigation: () => undefined,
    }), { initialProps: { engine: here } });
    await waitFor(() => expect(result.current.localShellProfiles.map((profile) => profile.id)).toEqual(["zsh"]));

    rerender({ engine: bastion });
    await waitFor(() => expect(result.current.localShellProfiles.map((profile) => profile.id)).toEqual(["bash"]));
  });

  it("opens a local folder with the requested shell profile and keeps failures on the current screen", async () => {
    const local: TerminalSession = { ...session("local"), kind: "shell" };
    delete local.alias;
    const opened: (TerminalSession | null)[] = [local, null];
    const open = vi.fn(async () => opened.shift() ?? null);
    const navigate = vi.fn();
    const { result } = renderHook(() => useTerminalWorkspaceController({
      api,
      shells,
      terminalSessions: { sessions: [local], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      enabled: true,
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

  it("falls back to the first session once the selected session is gone", async () => {
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
        api,
        shells,
        terminalSessions: {
          sessions: [session("s0"), session("s1")],
          refresh: vi.fn().mockResolvedValue(undefined),
          open,
        } as unknown as TerminalSessionsState,
        enabled: true,
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
      api,
      shells,
      terminalSessions: { sessions: [remote, local, unknown], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      enabled: true,
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
      api,
      shells,
      runTarget: "r540",
      terminalSessions: { sessions: [], refresh: vi.fn().mockResolvedValue(undefined), open } as unknown as TerminalSessionsState,
      enabled: true,
      section: "Home",
      navigate,
      closeNavigation: () => undefined,
    }));

    let opening: Promise<void> = Promise.resolve();
    act(() => { opening = result.current.openSSHSession("db"); });
    expect(navigate).toHaveBeenCalledWith("Terminal");
    await act(async () => { finish(null); await opening; });
  });
});

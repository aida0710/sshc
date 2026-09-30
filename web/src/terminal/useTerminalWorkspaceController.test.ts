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
    kind: "ssh",
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

function useController(list: TerminalSession[], refresh: () => Promise<void>, section: Section = "Terminal") {
  return useTerminalWorkspaceController({
    api,
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

  it("keeps the terminal screen behind another section while a workspace restore has no session yet", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useController([], refresh, "Connections"));
    expect(result.current.terminalScreenMounted).toBe(false);

    act(() => result.current.setWorkspaceRestoring(true));
    expect(result.current.terminalScreenMounted).toBe(true);

    act(() => result.current.setWorkspaceRestoring(false));
    expect(result.current.terminalScreenMounted).toBe(false);
  });

  it("shows an opened SSH session in the terminal section, and stays put when it could not be opened", async () => {
    const opened: (TerminalSession | null)[] = [session("s1"), null];
    const open = vi.fn(async () => opened.shift() ?? null);
    const navigate = vi.fn();
    const { result } = renderHook(() =>
      useTerminalWorkspaceController({
        api,
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
});

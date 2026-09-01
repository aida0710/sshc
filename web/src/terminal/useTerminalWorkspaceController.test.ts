import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { IntegrationsApi, TerminalSession } from "../api/integrations";
import type { TerminalSessionsState } from "./sessions";
import { useTerminalWorkspaceController } from "./useTerminalWorkspaceController";

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
} as unknown as IntegrationsApi;

function useController(list: TerminalSession[], refresh: () => Promise<void>) {
  return useTerminalWorkspaceController({
    api,
    consoles: {
      sessions: list,
      maxSessions: 50,
      busy: false,
      problem: "",
      loaded: true,
      refresh,
    } as unknown as TerminalSessionsState,
    enabled: true,
    section: "Terminal",
    navigate: () => undefined,
    closeNavigation: () => undefined,
  });
}

describe("useTerminalWorkspaceController", () => {
  it("keeps a freshly opened console selected while the session list catches up", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const listed = [session("s1"), session("s2")];
    const { result, rerender } = renderHook(
      ({ list }: { list: TerminalSession[] }) => useController(list, refresh),
      { initialProps: { list: listed } },
    );
    await waitFor(() => expect(result.current.activeConsole).toBe("s1"));

    // Quick Connect opens the session through the API, so the list still lacks it.
    act(() => result.current.showConsole("s3"));
    expect(result.current.activeConsole).toBe("s3");
    expect(refresh).toHaveBeenCalled();

    await act(async () => {
      rerender({ list: [...listed, session("s3")] });
    });
    expect(result.current.activeConsole).toBe("s3");
  });

  it("falls back to the first session once the selected console is gone", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const listed = [session("s1"), session("s2")];
    const { result, rerender } = renderHook(
      ({ list }: { list: TerminalSession[] }) => useController(list, refresh),
      { initialProps: { list: listed } },
    );
    await waitFor(() => expect(result.current.activeConsole).toBe("s1"));

    await act(async () => {
      result.current.showConsole("s2");
    });
    expect(result.current.activeConsole).toBe("s2");

    await act(async () => {
      rerender({ list: [session("s1")] });
    });
    expect(result.current.activeConsole).toBe("s1");
  });
});

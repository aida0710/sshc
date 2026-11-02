import { useEffect } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { OpenTerminalSessionResponse, TerminalSession } from "../api/terminalSessions";
import { terminalProblemKey, useTerminalSessions, type TerminalSessionsApi } from "./sessions";

const list = { sessions: [], maxSessions: 50 };

function api(overrides: Partial<TerminalSessionsApi> = {}): TerminalSessionsApi {
  return {
    terminalSessions: vi.fn().mockResolvedValue(list),
    openTerminalSession: vi.fn(),
    reconnectTerminalSession: vi.fn().mockResolvedValue(list),
    stopTerminalReconnect: vi.fn().mockResolvedValue(list),
    answerTerminalHostKey: vi.fn().mockResolvedValue(list),
    closeTerminalSession: vi.fn().mockResolvedValue(list),
    setTerminalSessionTitle: vi.fn().mockResolvedValue(list),
    moveTerminalSession: vi.fn().mockResolvedValue(list),
    ...overrides,
  };
}

const translate = ((key: string) => key) as never;

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useTerminalSessions", () => {
  it.each(["remote_engine_vault_locked", "remote_engine_incompatible", "remote_engine_unreachable"])(
    "holds the %s refusal apart from the problems of operations and clears it when the next listing succeeds",
    async (code) => {
      const refusal = new ApiError(code, 409, { code, message: "refused" });
      const terminalSessions = vi.fn().mockRejectedValueOnce(refusal).mockResolvedValue(list);
      const client = api({ terminalSessions });
      const { result } = renderHook(() => useTerminalSessions(client, translate));
      await waitFor(() => expect(result.current.loaded).toBe(true));
      expect(result.current.remoteRefusal).toBe(refusal);
      expect(result.current.problem).toBe("");
      await act(async () => { await result.current.refresh(); });
      expect(result.current.remoteRefusal).toBeNull();
    },
  );

  it("does not hold a failure that is not a run target refusal", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(new ApiError("internal_error", 500, null));
    const client = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.remoteRefusal).toBeNull();
  });

  it("hands out no session of the previous engine, not even in the render that switches to the next", async () => {
    const listed = (id: string) => ({
      sessions: [{ id, kind: "shell", workspaceId: "default", title: id, startedAt: "2026-10-29T00:00:00Z", state: "connected", problem: "" } as TerminalSession],
      maxSessions: 50,
    });
    const here = api({ terminalSessions: vi.fn().mockResolvedValue(listed("here")) });
    let answerBastion = (_answer: ReturnType<typeof listed>) => {};
    const bastion = api({ terminalSessions: vi.fn(() => new Promise<ReturnType<typeof listed>>((resolve) => { answerBastion = resolve; })) });
    const seen: string[][] = [];
    const { result, rerender } = renderHook(({ client }) => {
      const sessions = useTerminalSessions(client, translate);
      seen.push(sessions.sessions.map((session) => session.id));
      return sessions;
    }, { initialProps: { client: here } });
    await waitFor(() => expect(result.current.sessions.map((session) => session.id)).toEqual(["here"]));

    seen.length = 0;
    rerender({ client: bastion });
    expect(seen[0]).toEqual([]);
    expect(result.current.loaded).toBe(false);
    await act(async () => answerBastion(listed("bastion")));
    expect(result.current.sessions.map((session) => session.id)).toEqual(["bastion"]);
    expect(seen.flat()).not.toContain("here");
  });

  it("does not offer the previous engine's refusal when switching to another engine", async () => {
    const refused = api({ terminalSessions: vi.fn().mockRejectedValue(new ApiError("remote_engine_incompatible", 409, null)) });
    const available = api();
    const offer = vi.fn();
    const { result, rerender } = renderHook(({ client }) => {
      const sessions = useTerminalSessions(client, translate);
      useEffect(() => { if (sessions.remoteRefusal !== null) offer(client); }, [client, sessions.remoteRefusal]);
      return sessions;
    }, { initialProps: { client: refused } });
    await waitFor(() => expect(result.current.remoteRefusal?.code).toBe("remote_engine_incompatible"));
    rerender({ client: available });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(offer).toHaveBeenCalledWith(refused);
    expect(offer).not.toHaveBeenCalledWith(available);
    expect(result.current.remoteRefusal).toBeNull();
  });

  it("keeps polling an empty refused list through the engine restart until it recovers", async () => {
    vi.useFakeTimers();
    const terminalSessions = vi.fn()
      .mockRejectedValueOnce(new ApiError("remote_engine_incompatible", 409, null))
      .mockRejectedValueOnce(new ApiError("remote_engine_not_running", 502, null))
      .mockResolvedValue(list);
    const client = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.remoteRefusal?.code).toBe("remote_engine_incompatible");
    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    expect(terminalSessions).toHaveBeenCalledTimes(2);
    expect(result.current.remoteRefusal?.code).toBe("remote_engine_not_running");
    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    expect(terminalSessions).toHaveBeenCalledTimes(3);
    expect(result.current.remoteRefusal).toBeNull();
  });

  it("separates the session limit from a session that could not be opened", async () => {
    const openTerminalSession = vi
      .fn()
      .mockRejectedValueOnce(
        new ApiError("terminal_session_limit", 409, { code: "terminal_session_limit", message: "full" }),
      )
      .mockRejectedValueOnce(
        new ApiError("terminal_start_failed", 500, { code: "terminal_start_failed", message: "refused" }),
      );
    const inlineClient = api({ openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "shell" });
    });
    expect(result.current.problem).toBe("terminal.limitRefused");

    await act(async () => {
      await result.current.open({ kind: "shell" });
    });
    expect(result.current.problem).toBe("terminal.openFailed");
  });

  it("answers a host key question, and says nothing when the question was already answered in the terminal", async () => {
    const answerTerminalHostKey = vi
      .fn()
      .mockResolvedValueOnce(list)
      .mockRejectedValueOnce(new ApiError("terminal_no_host_key_question", 409, { code: "terminal_no_host_key_question", message: "closed" }));
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const inlineClient = api({ answerTerminalHostKey, terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      expect(await result.current.answerHostKey("a", "SHA256:offered", true)).toBe(true);
    });
    expect(answerTerminalHostKey).toHaveBeenLastCalledWith("a", { fingerprint: "SHA256:offered", accept: true });

    const listings = terminalSessions.mock.calls.length;
    await act(async () => {
      expect(await result.current.answerHostKey("a", "SHA256:offered", false)).toBe(false);
    });
    expect(result.current.problem).toBe("");
    expect(terminalSessions.mock.calls.length).toBeGreaterThan(listings);
  });

  it("stays busy until every parallel session open has finished", async () => {
    let resolveFirst: (value: OpenTerminalSessionResponse) => void = () => undefined;
    let resolveSecond: (value: OpenTerminalSessionResponse) => void = () => undefined;
    const first = new Promise<OpenTerminalSessionResponse>((resolve) => { resolveFirst = resolve; });
    const second = new Promise<OpenTerminalSessionResponse>((resolve) => { resolveSecond = resolve; });
    const openTerminalSession = vi.fn()
      .mockReturnValueOnce(first)
      .mockReturnValueOnce(second);
    const inlineClient = api({ openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let firstOpen: Promise<unknown> = Promise.resolve();
    let secondOpen: Promise<unknown> = Promise.resolve();
    act(() => {
      firstOpen = result.current.open({ kind: "shell" });
      secondOpen = result.current.open({ kind: "shell" });
    });
    await waitFor(() => expect(openTerminalSession).toHaveBeenCalledTimes(2));
    expect(result.current.busy).toBe(true);

    await act(async () => {
      resolveFirst({
        session: {
          id: "first", kind: "shell", workspaceId: "default", title: "first", startedAt: "2026-09-04T00:00:00Z",
          state: "connected", problem: "",
        },
        streamTicket: "first-ticket",
      });
      await firstOpen;
    });
    expect(result.current.busy).toBe(true);

    await act(async () => {
      resolveSecond({
        session: {
          id: "second", kind: "shell", workspaceId: "default", title: "second", startedAt: "2026-09-04T00:00:01Z",
          state: "connected", problem: "",
        },
        streamTicket: "second-ticket",
      });
      await secondOpen;
    });
    expect(result.current.busy).toBe(false);
  });

  it("keeps an opened session listed when a close answered before the session existed", async () => {
    const existing = {
      id: "old", kind: "shell", workspaceId: "default", title: "bash", startedAt: "2026-09-30T00:00:00Z", state: "connected", problem: "",
    } as const;
    const opened = {
      id: "new", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-09-30T00:00:01Z", state: "connected", problem: "",
    } as const;
    const pendingRead = new Promise(() => undefined);
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce({ sessions: [existing], maxSessions: 50 })
      .mockReturnValue(pendingRead);
    let create: (value: unknown) => void = () => undefined;
    const openTerminalSession = vi.fn(() => new Promise((resolve) => { create = resolve; }));
    let answerClose: (value: unknown) => void = () => undefined;
    const closeTerminalSession = vi.fn(() => new Promise((resolve) => { answerClose = resolve; }));
    const client = api({ terminalSessions, openTerminalSession, closeTerminalSession } as never);
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toEqual([existing]));

    act(() => {
      void result.current.open({ kind: "shell" });
      void result.current.close("old");
    });
    await waitFor(() => expect(closeTerminalSession).toHaveBeenCalledTimes(1));
    await act(async () => { create({ session: opened, streamTicket: "ticket" }); });
    expect(result.current.sessions).toEqual([existing, opened]);

    // The engine listed the sessions for this close before it created the new one.
    await act(async () => { answerClose({ sessions: [], maxSessions: 50 }); });

    expect(result.current.sessions.map((session) => session.id)).toContain("new");
  });

  it("hands back an opened session, already listed, while the list is still being read again", async () => {
    const opened = {
      id: "new", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-09-30T00:00:00Z", state: "connected", problem: "",
    } as const;
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce(list)
      .mockReturnValue(new Promise(() => undefined));
    const openTerminalSession = vi.fn().mockResolvedValue({ session: opened, streamTicket: "ticket" });
    const client = api({ terminalSessions, openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let handedBack: unknown = null;
    act(() => {
      void result.current.open({ kind: "shell" }).then((session) => { handedBack = session; });
    });

    await waitFor(() => expect(handedBack).toEqual(opened));
    expect(result.current.sessions).toEqual([opened]);
    expect(terminalSessions).toHaveBeenCalledTimes(2);
  });

  it("keeps an opened session listed when the list cannot be read afterwards", async () => {
    const opened = {
      id: "new", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-09-30T00:00:00Z", state: "connected", problem: "",
    } as const;
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce(list)
      .mockRejectedValue(new Error("offline"));
    const openTerminalSession = vi.fn().mockResolvedValue({ session: opened, streamTicket: "ticket" });
    const client = api({ terminalSessions, openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "shell" });
    });

    expect(result.current.sessions).toEqual([opened]);
  });

  it("reports a finished load even when the list could not be read", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(new Error("offline"));
    const inlineClient = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.sessions).toEqual([]);
  });

  it("reads nothing until it is allowed to", async () => {
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const client = api({ terminalSessions });
    const { result, rerender } = renderHook(
      ({ enabled }) => useTerminalSessions(client, translate, enabled),
      { initialProps: { enabled: false } },
    );

    expect(terminalSessions).not.toHaveBeenCalled();
    expect(result.current.loaded).toBe(false);

    rerender({ enabled: true });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(terminalSessions).toHaveBeenCalled();
  });

  it("reads an empty list again while the page is shown, so a session opened in another browser appears", async () => {
    vi.useFakeTimers();
    const opened = {
      id: "a", kind: "shell", workspaceId: "default", title: "shell",
      startedAt: "2026-11-01T13:33:52Z", state: "connected", problem: "",
    } as const;
    const terminalSessions = vi.fn().mockResolvedValueOnce(list).mockResolvedValue({ sessions: [opened], maxSessions: 50 });
    const client = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.sessions).toEqual([]);

    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    expect(result.current.sessions).toEqual([opened]);
  });

  it("does not read an empty list while the page is hidden", async () => {
    vi.useFakeTimers();
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const client = api({ terminalSessions });
    renderHook(() => useTerminalSessions(client, translate));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    const first = terminalSessions.mock.calls.length;

    await act(async () => { await vi.advanceTimersByTimeAsync(6_000); });
    expect(terminalSessions).toHaveBeenCalledTimes(first);
  });

  it("continues polling hidden sessions so background agent signals can be observed", async () => {
    vi.useFakeTimers();
    vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    const connected = {
      id: "a", kind: "ssh", workspaceId: "default", alias: "osaka", title: "osaka",
      startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
    } as const;
    const terminalSessions = vi.fn().mockResolvedValue({ sessions: [connected], maxSessions: 50 });
    const inlineClient = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));

    await act(async () => {
      vi.runAllTicks();
      await Promise.resolve();
    });
    expect(result.current.sessions).toEqual([connected]);
    const beforeInterval = terminalSessions.mock.calls.length;

    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    expect(terminalSessions.mock.calls.length).toBeGreaterThan(beforeInterval);
  });

  it("does not let an older refresh overwrite a newer session limit", async () => {
    let resolveInitial: (value: typeof list) => void = () => undefined;
    const initial = new Promise<typeof list>((resolve) => {
      resolveInitial = resolve;
    });
    const terminalSessions = vi
      .fn()
      .mockReturnValueOnce(initial)
      .mockResolvedValueOnce({ sessions: [], maxSessions: 1 });
    const inlineClient = api({ terminalSessions });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));

    await waitFor(() => expect(terminalSessions).toHaveBeenCalledTimes(1));
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.maxSessions).toBe(1);

    await act(async () => {
      resolveInitial({ sessions: [], maxSessions: 50 });
      await initial;
    });
    expect(result.current.maxSessions).toBe(1);
  });

  it("does not let an in-flight refresh resurrect a session after close", async () => {
    const connected = {
      id: "a", kind: "ssh", workspaceId: "default", alias: "bastion", title: "bastion",
      startedAt: "2026-08-26T01:00:00Z", state: "connected", problem: "",
    } as const;
    const beforeClose = { sessions: [connected], maxSessions: 50 };
    let resolveStale: (value: typeof beforeClose) => void = () => undefined;
    const stale = new Promise<typeof beforeClose>((resolve) => { resolveStale = resolve; });
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce(beforeClose)
      .mockReturnValueOnce(stale);
    const closeTerminalSession = vi.fn().mockResolvedValue({ sessions: [], maxSessions: 50 });
    const client = api({ terminalSessions, closeTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toEqual([connected]));

    let pendingRefresh: Promise<void> = Promise.resolve();
    act(() => { pendingRefresh = result.current.refresh(); });
    await waitFor(() => expect(terminalSessions).toHaveBeenCalledTimes(2));
    await act(async () => { await result.current.close("a"); });
    expect(result.current.sessions).toEqual([]);

    await act(async () => {
      resolveStale(beforeClose);
      await pendingRefresh;
    });
    expect(result.current.sessions).toEqual([]);
  });

  it("does not let parallel mutation responses resurrect a session", async () => {
    const first = {
      id: "a", kind: "ssh", workspaceId: "default", alias: "first", title: "first",
      startedAt: "2026-09-04T01:00:00Z", state: "connected", problem: "",
    } as const;
    const second = {
      id: "b", kind: "ssh", workspaceId: "default", alias: "second", title: "second",
      startedAt: "2026-09-04T01:00:01Z", state: "connected", problem: "",
    } as const;
    const initial = { sessions: [first, second], maxSessions: 50 };
    let resolveFirst: (value: { sessions: [typeof second]; maxSessions: number }) => void = () => undefined;
    let resolveSecond: (value: typeof list) => void = () => undefined;
    const firstClose = new Promise<{ sessions: [typeof second]; maxSessions: number }>((resolve) => {
      resolveFirst = resolve;
    });
    const secondClose = new Promise<typeof list>((resolve) => { resolveSecond = resolve; });
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce(initial)
      .mockResolvedValue(list);
    const closeTerminalSession = vi.fn()
      .mockReturnValueOnce(firstClose)
      .mockReturnValueOnce(secondClose);
    const client = api({ terminalSessions, closeTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toEqual([first, second]));

    let closingFirst: Promise<void> = Promise.resolve();
    let closingSecond: Promise<void> = Promise.resolve();
    act(() => {
      closingFirst = result.current.close("a");
      closingSecond = result.current.close("b");
    });
    resolveSecond(list);
    await act(async () => { await closingSecond; });
    expect(result.current.sessions).toEqual([]);

    // The older response arrives last with b still present. It is rejected and
    // followed by an authoritative list read instead of reviving b locally.
    resolveFirst({ sessions: [second], maxSessions: 50 });
    await act(async () => { await closingFirst; });

    expect(result.current.sessions).toEqual([]);
    expect(terminalSessions).toHaveBeenCalledTimes(2);
  });

  it("does not let an older close response erase a session opened afterwards", async () => {
    const first = {
      id: "a", kind: "ssh", workspaceId: "default", alias: "first", title: "first",
      startedAt: "2026-09-04T01:00:00Z", state: "connected", problem: "",
    } as const;
    const opened = {
      id: "b", kind: "shell", workspaceId: "default", title: "second",
      startedAt: "2026-09-04T01:00:01Z", state: "connected", problem: "",
    } as const;
    let resolveClose: (value: typeof list) => void = () => undefined;
    const closingResponse = new Promise<typeof list>((resolve) => { resolveClose = resolve; });
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce({ sessions: [first], maxSessions: 50 })
      .mockResolvedValue({ sessions: [opened], maxSessions: 50 });
    const client = api({
      terminalSessions,
      closeTerminalSession: vi.fn(() => closingResponse),
      openTerminalSession: vi.fn(async () => ({ session: opened, streamTicket: "ticket-b" })),
    });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toEqual([first]));

    let closing: Promise<void> = Promise.resolve();
    act(() => { closing = result.current.close("a"); });
    await act(async () => { await result.current.open({ kind: "shell" }); });
    expect(result.current.sessions).toEqual([opened]);

    resolveClose(list);
    await act(async () => { await closing; });

    expect(result.current.sessions).toEqual([opened]);
    expect(terminalSessions).toHaveBeenCalledTimes(3);
  });

  it("says so when a rename is refused and keeps the list it had", async () => {
    const setTerminalSessionTitle = vi.fn().mockRejectedValue(
      new ApiError("invalid_terminal_title", 400, { code: "invalid_terminal_title", message: "no" }),
    );
    const inlineClient = api({ setTerminalSessionTitle });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let outcome = true;
    await act(async () => {
      outcome = await result.current.rename("a", "  ");
    });

    expect(outcome).toBe(false);
    expect(result.current.problem).toBe("terminal.renameFailed");
  });

  it("reconnects the same retained session and adopts the returned state", async () => {
    const connected = {
      id: "a", kind: "ssh", workspaceId: "default", alias: "bastion", title: "bastion",
      startedAt: "2026-08-26T01:00:00Z", state: "connected", problem: "",
    } as const;
    const reconnectTerminalSession = vi.fn().mockResolvedValue({ sessions: [connected], maxSessions: 50 });
    const client = api({ reconnectTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let outcome = false;
    await act(async () => {
      outcome = await result.current.reconnect("a");
    });

    expect(outcome).toBe(true);
    expect(reconnectTerminalSession).toHaveBeenCalledWith("a");
    expect(result.current.sessions).toEqual([connected]);
  });

  it("reports a refused explicit reconnect and refreshes the retained state", async () => {
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const reconnectTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("host_key_changed", 422, { code: "host_key_changed", message: "changed" }),
    );
    const client = api({ terminalSessions, reconnectTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    let outcome = true;
    await act(async () => {
      outcome = await result.current.reconnect("a");
    });

    expect(outcome).toBe(false);
    expect(result.current.problem).toBe("terminal.hostKeyChanged");
    expect(terminalSessions).toHaveBeenCalledTimes(2);
  });
  it("names why a connection the configuration does not allow was refused", async () => {
    const openTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("jump_depth_exceeded", 422, { code: "jump_depth_exceeded", message: "no" }),
    );
    const inlineClient = api({ openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "ssh", alias: "jump" });
    });
    expect(result.current.problem).toBe("terminal.jumpDepthExceeded");
  });

  it("falls back to the plain refusal for a code it does not know", async () => {
    const openTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("something_new", 500, { code: "something_new", message: "no" }),
    );
    const inlineClient = api({ openTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(inlineClient, translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "ssh", alias: "jump" });
    });
    expect(result.current.problem).toBe("terminal.openFailed");
  });

  it("closes every session once", async () => {
    const server = [
      { id: "a", live: true },
      { id: "b", live: true },
    ];
    const listing = () => ({
      sessions: server.map(({ id }) => ({ id })) as never,
      maxSessions: 50,
    });
    const closeTerminalSession = vi.fn(async (id: string) => {
      const found = server.find((session) => session.id === id);
      if (found === undefined) return listing();
      server.splice(server.indexOf(found), 1);
      return listing();
    });
    const client = api({ terminalSessions: vi.fn(async () => listing()), closeTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toHaveLength(2));

    await act(async () => {
      await result.current.closeAll();
    });

    expect(closeTerminalSession).toHaveBeenCalledTimes(2);
    expect(result.current.sessions).toEqual([]);
    expect(result.current.problem).toBe("");
  });

  it("does not call a close a failure when the session was already gone", async () => {
    const one = [{ id: "a" }] as never;
    const terminalSessions = vi.fn()
      .mockResolvedValueOnce({ sessions: one, maxSessions: 50 })
      .mockResolvedValue(list);
    const closeTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("terminal_session_not_found", 404, { code: "terminal_session_not_found", message: "gone" }),
    );
    const client = api({ terminalSessions, closeTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    await act(async () => {
      await result.current.closeAll();
    });

    expect(closeTerminalSession).toHaveBeenCalledTimes(1);
    expect(result.current.sessions).toEqual([]);
    expect(result.current.problem).toBe("");
  });

  it("says so when one of them refuses to close", async () => {
    const one = [{ id: "a" }] as never;
    const client = api({
      terminalSessions: vi.fn().mockResolvedValue({ sessions: one, maxSessions: 50 }),
      closeTerminalSession: vi.fn().mockRejectedValue(new Error("refused")),
    });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    await act(async () => {
      await result.current.closeAll();
    });

    expect(result.current.problem).toBe("terminal.closeFailed");
    expect(result.current.sessions).toHaveLength(1);
  });
});

describe("terminalProblemKey", () => {
  it("says the VPN route is why reconnecting stopped, not that the terminal failed to open", () => {
    expect(terminalProblemKey("vpn_route_refused")).toBe("terminal.vpnRouteRefused");
  });

  it("says that the VPN route was disconnected when that cancelled the connection", () => {
    expect(terminalProblemKey("vpn_route_disconnected")).toBe("terminal.vpnRouteDisconnected");
  });

  it("says a connection that never connected failed, not that it is reconnecting", () => {
    expect(terminalProblemKey("connect_failed")).toBe("terminal.connectFailed");
  });

  it("says the server rejected authentication, not that the terminal failed to open", () => {
    expect(terminalProblemKey("authentication_rejected")).toBe("terminal.authenticationRejected");
  });

  it("says the route settings conflict when the VPN, ProxyJump and ProxyCommand cannot be combined", () => {
    expect(terminalProblemKey("route_misconfigured")).toBe("terminal.routeMisconfigured");
  });

  it("says the folder cannot be opened in a terminal when its name has control characters", () => {
    expect(terminalProblemKey("remote_working_directory_unsupported")).toBe(
      "terminal.remoteWorkingDirectoryUnsupported",
    );
  });

  it("says the engine folder cannot be used when the local shell cannot start there", () => {
    expect(terminalProblemKey("local_working_directory_unavailable")).toBe("terminal.localWorkingDirectoryUnavailable");
  });

  it("says that a symlinked known_hosts could not save the host key, not that the terminal failed to open", () => {
    expect(terminalProblemKey("known_hosts_symlink")).toBe("terminal.knownHostsSymlink");
  });
});

describe("a session's stream reporting its exit", () => {
  it("shows the status the stream reported at once, not 0 until the next listing", async () => {
    const running: TerminalSession = {
      id: "a", kind: "ssh", alias: "bastion", workspaceId: "default", title: "bastion", startedAt: "2026-10-28T09:00:00Z", state: "connected", problem: "",
    };
    const client = api({ terminalSessions: vi.fn().mockResolvedValue({ sessions: [running], maxSessions: 50 }) });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    act(() => result.current.markExited("a", { code: -1, signal: "", connectionLost: true }));

    expect(result.current.sessions[0]).toMatchObject({ state: "exited", exited: { code: -1, signal: "", connectionLost: true } });
  });
});

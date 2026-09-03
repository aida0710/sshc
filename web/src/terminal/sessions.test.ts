import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { OpenTerminalSessionResponse } from "../api/integrations";
import { useTerminalSessions, type TerminalSessionsApi } from "./sessions";

const list = { sessions: [], maxSessions: 50 };

function api(overrides: Partial<TerminalSessionsApi> = {}): TerminalSessionsApi {
  return {
    terminalSessions: vi.fn().mockResolvedValue(list),
    openTerminalSession: vi.fn(),
    reconnectTerminalSession: vi.fn().mockResolvedValue(list),
    closeTerminalSession: vi.fn().mockResolvedValue(list),
    renameTerminalSession: vi.fn().mockResolvedValue(list),
    ...overrides,
  } as TerminalSessionsApi;
}

const translate = ((key: string) => key) as never;

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useTerminalSessions", () => {
  it("separates the session limit from a console that could not be opened", async () => {
    const openTerminalSession = vi
      .fn()
      .mockRejectedValueOnce(
        new ApiError("terminal_session_limit", 409, { code: "terminal_session_limit", message: "full" }),
      )
      .mockRejectedValueOnce(
        new ApiError("terminal_start_failed", 500, { code: "terminal_start_failed", message: "refused" }),
      );
    const { result } = renderHook(() => useTerminalSessions(api({ openTerminalSession }), translate));
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

  it("stays busy until every parallel session open has finished", async () => {
    let resolveFirst: (value: OpenTerminalSessionResponse) => void = () => undefined;
    let resolveSecond: (value: OpenTerminalSessionResponse) => void = () => undefined;
    const first = new Promise<OpenTerminalSessionResponse>((resolve) => { resolveFirst = resolve; });
    const second = new Promise<OpenTerminalSessionResponse>((resolve) => { resolveSecond = resolve; });
    const openTerminalSession = vi.fn()
      .mockReturnValueOnce(first)
      .mockReturnValueOnce(second);
    const { result } = renderHook(() => useTerminalSessions(api({ openTerminalSession }), translate));
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
          id: "first", kind: "shell", title: "first", startedAt: "2026-09-04T00:00:00Z",
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
          id: "second", kind: "shell", title: "second", startedAt: "2026-09-04T00:00:01Z",
          state: "connected", problem: "",
        },
        streamTicket: "second-ticket",
      });
      await secondOpen;
    });
    expect(result.current.busy).toBe(false);
  });

  it("reports a finished load even when the list could not be read", async () => {
    const terminalSessions = vi.fn().mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useTerminalSessions(api({ terminalSessions }), translate));

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.sessions).toEqual([]);
  });

  it("reads nothing until it is allowed to", async () => {
    const terminalSessions = vi.fn().mockResolvedValue(list);
    const { result, rerender } = renderHook(
      ({ enabled }) => useTerminalSessions(api({ terminalSessions }), translate, enabled),
      { initialProps: { enabled: false } },
    );

    expect(terminalSessions).not.toHaveBeenCalled();
    expect(result.current.loaded).toBe(false);

    rerender({ enabled: true });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(terminalSessions).toHaveBeenCalled();
  });

  it("continues polling hidden sessions so background agent signals can be observed", async () => {
    vi.useFakeTimers();
    vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    const connected = {
      id: "a", kind: "ssh", alias: "osaka", title: "osaka",
      startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
    } as const;
    const terminalSessions = vi.fn().mockResolvedValue({ sessions: [connected], maxSessions: 50 });
    const { result } = renderHook(() => useTerminalSessions(api({ terminalSessions }), translate));

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
    const { result } = renderHook(() => useTerminalSessions(api({ terminalSessions }), translate));

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
      id: "a", kind: "ssh", alias: "bastion", title: "bastion",
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
      id: "a", kind: "ssh", alias: "first", title: "first",
      startedAt: "2026-09-04T01:00:00Z", state: "connected", problem: "",
    } as const;
    const second = {
      id: "b", kind: "ssh", alias: "second", title: "second",
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
      id: "a", kind: "ssh", alias: "first", title: "first",
      startedAt: "2026-09-04T01:00:00Z", state: "connected", problem: "",
    } as const;
    const opened = {
      id: "b", kind: "shell", title: "second",
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
    const renameTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("invalid_terminal_title", 400, { code: "invalid_terminal_title", message: "no" }),
    );
    const { result } = renderHook(() => useTerminalSessions(api({ renameTerminalSession }), translate));
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
      id: "a", kind: "ssh", alias: "bastion", title: "bastion",
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
      new ApiError("proxy_command_with_jump", 422, { code: "proxy_command_with_jump", message: "no" }),
    );
    const { result } = renderHook(() => useTerminalSessions(api({ openTerminalSession }), translate));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await act(async () => {
      await result.current.open({ kind: "ssh", alias: "jump" });
    });
    expect(result.current.problem).toBe("terminal.proxyCommandWithJump");
  });

  it("falls back to the plain refusal for a code it does not know", async () => {
    const openTerminalSession = vi.fn().mockRejectedValue(
      new ApiError("something_new", 500, { code: "something_new", message: "no" }),
    );
    const { result } = renderHook(() => useTerminalSessions(api({ openTerminalSession }), translate));
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

  it("keeps trying while a session takes several attempts to die", async () => {
    const server = [{ id: "a" }];
    const listing = () => ({ sessions: [...server] as never, maxSessions: 50 });
    const closeTerminalSession = vi.fn(async () => {
      if (closeTerminalSession.mock.calls.length >= 3) server.length = 0;
      return listing();
    });
    const client = api({ terminalSessions: vi.fn(async () => listing()), closeTerminalSession });
    const { result } = renderHook(() => useTerminalSessions(client, translate));
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    await act(async () => {
      await result.current.closeAll();
    });

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

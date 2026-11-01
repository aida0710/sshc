import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspacesApi } from "../api/terminalWorkspaces";
import { useTerminalWorkspaces } from "./useTerminalWorkspaces";

const translate = ((key: string) => key) as never;
const main = { id: "default", name: "", hidden: false, builtIn: true };
const dev = { id: "w1", name: "dev", hidden: false, builtIn: false };

afterEach(() => {
  vi.useRealTimers();
});

// One array, as the session list hands out, so that renders do not read the list again by themselves.
const noSessions: TerminalSession[] = [];
const onSessionsMoved = vi.fn();

function renderWorkspaces(api: TerminalWorkspacesApi) {
  return renderHook(() => useTerminalWorkspaces({ sessions: noSessions, api, translate, enabled: true, onSessionsMoved }));
}

it("recovers missing workspaces after a successful empty session listing", async () => {
  const workspaces = [{ id: "default", name: "", hidden: false, builtIn: true }];
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockRejectedValueOnce(new ApiError("remote_engine_incompatible", 409, null))
      .mockResolvedValue({ workspaces, maxWorkspaces: 50 }),
    create: vi.fn(), update: vi.fn(), remove: vi.fn(), move: vi.fn(),
  };
  const translate = ((key: string) => key) as never;
  const onSessionsMoved = vi.fn();
  const { result, rerender } = renderHook(({ sessions }: { sessions: TerminalSession[] }) => useTerminalWorkspaces({
    sessions, api, translate, enabled: true, onSessionsMoved,
  }), { initialProps: { sessions: [] } });
  await waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
  expect(result.current.workspaces).toEqual([]);
  rerender({ sessions: [] });
  await waitFor(() => expect(result.current.workspaces).toEqual(workspaces));
  rerender({ sessions: [] });
  expect(api.list).toHaveBeenCalledTimes(2);
});

it("draws the tree in the order the engine answers after a move, and reads the list again when the move is refused", async () => {
  const listed = [
    { id: "default", name: "", hidden: false, builtIn: true },
    { id: "w1", name: "dev", hidden: false, builtIn: false },
  ];
  const moved = [listed[1]!, listed[0]!];
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockResolvedValue({ workspaces: listed, maxWorkspaces: 50 }),
    create: vi.fn(), update: vi.fn(), remove: vi.fn(),
    move: vi.fn().mockResolvedValueOnce({ workspaces: moved, maxWorkspaces: 50 })
      .mockRejectedValueOnce(new ApiError("terminal_workspace_not_found", 404, null)),
  };
  const translate = ((key: string) => key) as never;
  const { result } = renderHook(() => useTerminalWorkspaces({
    sessions: [], api, translate, enabled: true, onSessionsMoved: vi.fn(),
  }));
  await waitFor(() => expect(result.current.workspaces).toEqual(listed));

  await act(async () => {
    expect(await result.current.move("w1", "default")).toBe(true);
  });
  expect(api.move).toHaveBeenCalledWith("w1", "default");
  expect(result.current.workspaces).toEqual(moved);

  await act(async () => {
    expect(await result.current.move("gone", null)).toBe(false);
  });
  expect(result.current.problem).toBe("workspace.notFound");
  expect(result.current.workspaces).toEqual(listed);
});

it("reads the list again on the session list's schedule, so a workspace made in another browser appears", async () => {
  vi.useFakeTimers();
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockResolvedValueOnce({ workspaces: [main], maxWorkspaces: 50 })
      .mockResolvedValue({ workspaces: [main, dev], maxWorkspaces: 50 }),
    create: vi.fn(), update: vi.fn(), remove: vi.fn(), move: vi.fn(),
  };
  const { result } = renderWorkspaces(api);
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  expect(result.current.workspaces).toEqual([main]);

  await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
  expect(result.current.workspaces).toEqual([main, dev]);
});

it("keeps a new name when a read that was on its way during the rename answers afterwards", async () => {
  vi.useFakeTimers();
  let answerSlowRead: (value: { workspaces: typeof main[]; maxWorkspaces: number }) => void = () => undefined;
  const renamed = { ...dev, name: "sshc dev" };
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockResolvedValueOnce({ workspaces: [main, dev], maxWorkspaces: 50 })
      .mockImplementationOnce(() => new Promise((resolve) => { answerSlowRead = resolve; }))
      .mockResolvedValue({ workspaces: [main, renamed], maxWorkspaces: 50 }),
    create: vi.fn(), update: vi.fn().mockResolvedValue(renamed), remove: vi.fn(), move: vi.fn(),
  };
  const { result } = renderWorkspaces(api);
  await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
  expect(api.list).toHaveBeenCalledTimes(2);

  await act(async () => {
    expect(await result.current.update("w1", { name: "sshc dev" })).toBe(true);
  });
  await act(async () => {
    answerSlowRead({ workspaces: [main, dev], maxWorkspaces: 50 });
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.workspaces).toEqual([main, renamed]);
});

it("keeps the tree and says nothing when a scheduled read fails", async () => {
  vi.useFakeTimers();
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockResolvedValueOnce({ workspaces: [main, dev], maxWorkspaces: 50 })
      .mockRejectedValue(new ApiError("internal", 500, null)),
    create: vi.fn(), update: vi.fn(), remove: vi.fn(), move: vi.fn(),
  };
  const { result } = renderWorkspaces(api);
  await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(result.current.workspaces).toEqual([main, dev]);
  expect(result.current.problem).toBe("");
});

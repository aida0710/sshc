import { renderHook, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspacesApi } from "../api/terminalWorkspaces";
import { useTerminalWorkspaces } from "./useTerminalWorkspaces";

it("recovers missing workspaces after a successful empty session listing", async () => {
  const workspaces = [{ id: "default", name: "", hidden: false, builtIn: true }];
  const api: TerminalWorkspacesApi = {
    list: vi.fn().mockRejectedValueOnce(new ApiError("remote_engine_incompatible", 409, null))
      .mockResolvedValue({ workspaces, maxWorkspaces: 50 }),
    create: vi.fn(), update: vi.fn(), remove: vi.fn(),
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

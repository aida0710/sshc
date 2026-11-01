import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import {
  terminalWorkspacesApi,
  type TerminalWorkspace,
  type TerminalWorkspacesApi,
  type UpdateTerminalWorkspaceRequest,
} from "../api/terminalWorkspaces";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { usePolling } from "../ui/usePolling";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { sessionListPollIntervalMs } from "./sessions";

export type TerminalWorkspacesState = {
  workspaces: TerminalWorkspace[];
  maxWorkspaces: number;
  problem: string;
  create: (name: string) => Promise<TerminalWorkspace | null>;
  update: (id: string, change: UpdateTerminalWorkspaceRequest) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
  move: (id: string, beforeId: string | null) => Promise<boolean>;
};

// The engine's terminal workspaces. Another browser, a machine that uses this
// engine as its run target, or `sshc` can create, rename, reorder or delete
// one, so the list is read again on the session list's schedule while the page
// is shown, and at once when a session names a workspace it does not hold.
export function useTerminalWorkspaces({
  sessions,
  translate,
  enabled,
  onSessionsMoved,
  api = terminalWorkspacesApi,
}: {
  sessions: TerminalSession[];
  translate: Translate;
  enabled: boolean;
  // Deleting a workspace moves its sessions to the default one.
  onSessionsMoved: () => Promise<void>;
  api?: TerminalWorkspacesApi;
}): TerminalWorkspacesState {
  const [workspaces, setWorkspaces] = useState<TerminalWorkspace[]>([]);
  const [maxWorkspaces, setMaxWorkspaces] = useState(0);
  const [problem, setProblem] = useState("");
  // A change answers with the engine's state after it, so it retires the reads
  // still on their way: a slow read must not bring back an old name or order.
  const listGeneration = useRequestGeneration();

  // A different api is another engine, whose workspaces are read afresh.
  useEffect(() => {
    listGeneration.retire();
    setWorkspaces([]);
    setMaxWorkspaces(0);
    setProblem("");
  }, [api, listGeneration]);

  const readList = useCallback(async () => {
    const isCurrent = listGeneration.begin();
    const listed = await api.list();
    if (!isCurrent()) return;
    setWorkspaces(listed.workspaces);
    setMaxWorkspaces(listed.maxWorkspaces);
  }, [api, listGeneration]);

  const reload = useCallback(async () => {
    try {
      await readList();
    } catch (error) {
      // A run-target engine that cannot be reached is reported once, by the
      // session list beside this one.
      const code = failureCode(error);
      if (!code.startsWith("remote_engine_")) setProblem(translate(workspaceProblemKey(code)));
    }
  }, [readList, translate]);

  // A scheduled read that fails keeps the tree as it is and says nothing: the
  // session list beside it already reports an engine it cannot reach.
  usePolling(() => readList().catch(() => undefined), { intervalMs: sessionListPollIntervalMs, enabled });

  const known = new Set(workspaces.map((workspace) => workspace.id));
  const unknownWorkspace = sessions.some((session) => !known.has(session.workspaceId));
  // A successful session listing also retries missing workspaces after a
  // remote engine recovers, even when that engine has no sessions yet.
  useEffect(() => {
    if (enabled && (workspaces.length === 0 || unknownWorkspace)) void reload();
  }, [enabled, reload, sessions, unknownWorkspace, workspaces.length]);

  const run = useCallback(async <T,>(request: () => Promise<T>): Promise<T | null> => {
    setProblem("");
    try {
      return await request();
    } catch (error) {
      setProblem(translate(workspaceProblemKey(failureCode(error))));
      return null;
    }
  }, [translate]);

  const create = useCallback(async (name: string) => {
    const created = await run(() => api.create(name));
    // The engine decides where a new workspace sits (before the hidden agents
    // one), so the list is read again rather than appended to.
    if (created !== null) await reload();
    return created;
  }, [api, reload, run]);

  const update = useCallback(async (id: string, change: UpdateTerminalWorkspaceRequest) => {
    const updated = await run(() => api.update(id, change));
    if (updated === null) return false;
    listGeneration.retire();
    setWorkspaces((current) => current.map((workspace) => workspace.id === id ? updated : workspace));
    return true;
  }, [api, listGeneration, run]);

  const remove = useCallback(async (id: string) => {
    const listed = await run(() => api.remove(id));
    if (listed === null) return false;
    listGeneration.retire();
    setWorkspaces(listed.workspaces);
    await onSessionsMoved();
    return true;
  }, [api, listGeneration, onSessionsMoved, run]);

  const move = useCallback(async (id: string, beforeId: string | null) => {
    const listed = await run(() => api.move(id, beforeId));
    if (listed === null) {
      // The workspace or its neighbour is gone (another tab deleted it), so the tree is drawn from the
      // engine's list again.
      await reload();
      return false;
    }
    listGeneration.retire();
    setWorkspaces(listed.workspaces);
    return true;
  }, [api, listGeneration, reload, run]);

  return { workspaces, maxWorkspaces, problem, create, update, remove, move };
}

function workspaceProblemKey(code: string): MessageKey {
  switch (code) {
    case "terminal_workspace_limit":
      return "workspace.limitReached";
    case "invalid_terminal_workspace_name":
      return "workspace.invalidName";
    case "terminal_workspace_not_found":
      return "workspace.notFound";
    default:
      return "workspace.changeFailed";
  }
}

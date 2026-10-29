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

export type TerminalWorkspacesState = {
  workspaces: TerminalWorkspace[];
  maxWorkspaces: number;
  problem: string;
  create: (name: string) => Promise<TerminalWorkspace | null>;
  update: (id: string, change: UpdateTerminalWorkspaceRequest) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
  move: (id: string, beforeId: string | null) => Promise<boolean>;
};

// The engine's terminal workspaces. The list is read again whenever a session
// names a workspace it does not hold: another tab or `sshc` made one.
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

  // A different api is another engine, whose workspaces are read afresh.
  useEffect(() => {
    setWorkspaces([]);
    setMaxWorkspaces(0);
    setProblem("");
  }, [api]);

  const reload = useCallback(async () => {
    try {
      const listed = await api.list();
      setWorkspaces(listed.workspaces);
      setMaxWorkspaces(listed.maxWorkspaces);
    } catch (error) {
      // A run-target engine that cannot be reached is reported once, by the
      // session list beside this one.
      const code = failureCode(error);
      if (!code.startsWith("remote_engine_")) setProblem(translate(workspaceProblemKey(code)));
    }
  }, [api, translate]);

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
    setWorkspaces((current) => current.map((workspace) => workspace.id === id ? updated : workspace));
    return true;
  }, [api, run]);

  const remove = useCallback(async (id: string) => {
    const listed = await run(() => api.remove(id));
    if (listed === null) return false;
    setWorkspaces(listed.workspaces);
    await onSessionsMoved();
    return true;
  }, [api, onSessionsMoved, run]);

  const move = useCallback(async (id: string, beforeId: string | null) => {
    const listed = await run(() => api.move(id, beforeId));
    if (listed === null) {
      // The workspace or its neighbour is gone (another tab deleted it), so the tree is drawn from the
      // engine's list again.
      await reload();
      return false;
    }
    setWorkspaces(listed.workspaces);
    return true;
  }, [api, reload, run]);

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

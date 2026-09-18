import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import { failureCode } from "../../api/client";
import type { TerminalSession } from "../../api/terminalSessions";
import { workspaceApi } from "./api";
import { reduceLayout, restoreLayout, type LayoutState } from "./layout";
import { visit } from "./panes";

export type WorkspaceRestoreRequest = { id: string; sequence: number };

type RestoreAttempt = {
  generation: number;
  openedSessionIDs: Set<string>;
  cleanup: Promise<void>;
};

// Reopens a saved workspace: one session per pane, connected in parallel.
// Only the latest attempt may touch the layout; an attempt overtaken by a
// newer one closes every session it opened instead.
export function useWorkspaceRestore({
  restoreRequest,
  onRestoreConsumed,
  onOpenAlias,
  onOpenShell,
  onClose,
  onActive,
  onBegin,
  setLayout,
  report,
}: {
  restoreRequest: WorkspaceRestoreRequest | null;
  onRestoreConsumed: (sequence: number) => void;
  onOpenAlias: (alias: string) => Promise<TerminalSession | null>;
  onOpenShell: () => Promise<TerminalSession | null>;
  onClose: (id: string) => Promise<void>;
  onActive: (id: string) => void;
  // Called once the saved layout is fetched, before panes start connecting.
  onBegin: (id: string) => void;
  setLayout: Dispatch<SetStateAction<LayoutState | null>>;
  // The outcome for the problem banner; "" clears it.
  report: (problem: string) => void;
}) {
  const consumed = useRef(0);
  const generationRef = useRef(0);
  const activeAttempt = useRef<RestoreAttempt | null>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);

  // A superseded restore no longer owns visible panes, so every session it
  // created must be retired. Chain closes to keep mutation listings ordered.
  const retireSession = useCallback((attempt: RestoreAttempt, id: string): Promise<void> => {
    if (!attempt.openedSessionIDs.delete(id)) return attempt.cleanup;
    attempt.cleanup = attempt.cleanup
      .then(() => onCloseRef.current(id))
      .catch(() => undefined);
    return attempt.cleanup;
  }, []);
  const retireAttempt = useCallback((attempt: RestoreAttempt): Promise<void> => {
    for (const id of [...attempt.openedSessionIDs]) void retireSession(attempt, id);
    return attempt.cleanup;
  }, [retireSession]);
  useEffect(() => () => {
    generationRef.current += 1;
    const attempt = activeAttempt.current;
    activeAttempt.current = null;
    if (attempt !== null) void retireAttempt(attempt);
  }, [retireAttempt]);

  const restoreWorkspace = useCallback(async (id: string) => {
    if (id === "") return;
    // Increment before the first await so reverse-order restore responses can
    // never install an older layout over the user's latest choice.
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    const previous = activeAttempt.current;
    const attempt: RestoreAttempt = { generation, openedSessionIDs: new Set(), cleanup: Promise.resolve() };
    activeAttempt.current = attempt;
    if (previous !== null) void retireAttempt(previous);
    const current = () => attempt.generation === generationRef.current;
    const settle = (problem: string) => {
      if (activeAttempt.current === attempt) activeAttempt.current = null;
      attempt.openedSessionIDs.clear();
      report(problem);
    };
    try {
      const stored = await workspaceApi.restore(id);
      if (!current()) return;
      onBegin(id);
      let restored = restoreLayout(stored.layout, stored.focusedPaneId);
      const panes: { id: string; alias: string; kind?: "shell" }[] = [];
      visit(stored.layout, (pane) => {
        panes.push(pane);
        restored = reduceLayout(restored, { type: "connection-starting", paneId: pane.id });
      });
      setLayout(restored);
      await Promise.all(panes.map(async (pane) => {
        const session = pane.kind === "shell" ? await onOpenShell() : await onOpenAlias(pane.alias);
        if (session === null) {
          if (!current()) return;
          setLayout((layout) => layout === null ? layout : reduceLayout(layout, {
            type: "connection-failed", paneId: pane.id, problem: "open_failed",
          }));
          return;
        }
        attempt.openedSessionIDs.add(session.id);
        if (!current()) {
          await retireSession(attempt, session.id);
          return;
        }
        setLayout((layout) => layout === null ? layout : reduceLayout(layout, {
          type: "connection-started", paneId: pane.id, sessionId: session.id,
        }));
        if (pane.id === stored.focusedPaneId) onActive(session.id);
      }));
      if (!current()) {
        await retireAttempt(attempt);
        return;
      }
      settle("");
    } catch (error) {
      if (!current()) {
        await retireAttempt(attempt);
        return;
      }
      settle(failureCode(error) || "workspace_failed");
    }
  }, [onActive, onBegin, onOpenAlias, onOpenShell, report, retireAttempt, retireSession, setLayout]);

  useEffect(() => {
    if (restoreRequest === null || restoreRequest.sequence <= consumed.current) return;
    consumed.current = restoreRequest.sequence;
    onRestoreConsumed(restoreRequest.sequence);
    void restoreWorkspace(restoreRequest.id);
  }, [onRestoreConsumed, restoreRequest, restoreWorkspace]);

  return restoreWorkspace;
}

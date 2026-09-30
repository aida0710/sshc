import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { workspaceApi } from "./api";
import { reduceLayout, restoreLayout, type LayoutState } from "./layout";
import { visit } from "./panes";
import { describeWorkspaceFailure } from "./workspaceProblem";

export type WorkspaceRestoreRequest = { id: string; sequence: number };

type RestoreAttempt = {
  generation: number;
  openedSessionIds: Set<string>;
  cleanup: Promise<void>;
};

// Reopens a saved workspace: one session per pane, connected in parallel.
// Only the latest attempt may touch the layout; an attempt overtaken by a
// newer one closes every session it opened instead.
export function useWorkspaceRestore({
  restoreRequest,
  onRestoreConsumed,
  onRestoringChange,
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
  // Unmounting abandons a restore and closes the sessions it opened, so the
  // owner keeps this hook mounted while it reports true.
  onRestoringChange: (restoring: boolean) => void;
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
  const t = useTranslate();
  const consumed = useRef(0);
  const generationRef = useRef(0);
  const activeAttempt = useRef<RestoreAttempt | null>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);
  const onRestoringChangeRef = useRef(onRestoringChange);
  useEffect(() => { onRestoringChangeRef.current = onRestoringChange; }, [onRestoringChange]);

  // A superseded restore no longer owns visible panes, so every session it
  // created must be retired. Chain closes to keep mutation listings ordered.
  const retireSession = useCallback((attempt: RestoreAttempt, id: string): Promise<void> => {
    if (!attempt.openedSessionIds.delete(id)) return attempt.cleanup;
    attempt.cleanup = attempt.cleanup
      .then(() => onCloseRef.current(id))
      .catch(() => undefined);
    return attempt.cleanup;
  }, []);
  const retireAttempt = useCallback((attempt: RestoreAttempt): Promise<void> => {
    for (const id of [...attempt.openedSessionIds]) void retireSession(attempt, id);
    return attempt.cleanup;
  }, [retireSession]);
  useEffect(() => () => {
    generationRef.current += 1;
    const attempt = activeAttempt.current;
    activeAttempt.current = null;
    if (attempt === null) return;
    void retireAttempt(attempt);
    onRestoringChangeRef.current(false);
  }, [retireAttempt]);

  const restoreWorkspace = useCallback(async (id: string) => {
    if (id === "") return;
    // Increment before the first await so reverse-order restore responses can
    // never install an older layout over the user's latest choice.
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    const previous = activeAttempt.current;
    const attempt: RestoreAttempt = { generation, openedSessionIds: new Set(), cleanup: Promise.resolve() };
    activeAttempt.current = attempt;
    if (previous !== null) void retireAttempt(previous);
    onRestoringChangeRef.current(true);
    const current = () => attempt.generation === generationRef.current;
    const settle = (problem: string) => {
      if (activeAttempt.current === attempt) {
        activeAttempt.current = null;
        onRestoringChangeRef.current(false);
      }
      attempt.openedSessionIds.clear();
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
        attempt.openedSessionIds.add(session.id);
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
      settle(describeWorkspaceFailure(t, error));
    }
  }, [onActive, onBegin, onOpenAlias, onOpenShell, report, retireAttempt, retireSession, setLayout, t]);

  useEffect(() => {
    if (restoreRequest === null || restoreRequest.sequence <= consumed.current) return;
    consumed.current = restoreRequest.sequence;
    onRestoreConsumed(restoreRequest.sequence);
    void restoreWorkspace(restoreRequest.id);
  }, [onRestoreConsumed, restoreRequest, restoreWorkspace]);

  return restoreWorkspace;
}

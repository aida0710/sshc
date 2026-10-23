import { useCallback, useEffect, useMemo, useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import { emptySplitView, maximumSplitViewPanes, paneSessionIds, reduceSplitView, type PaneEdge, type SplitViewAction } from "./layout";

export function useSplitView({ sessions, runTarget }: { sessions: TerminalSession[]; runTarget: string | null }) {
  const [current, setCurrent] = useState({ runTarget, state: emptySplitView });
  const compact = useMediaQuery(mobileViewportQuery);
  // A target change must discard the old engine's layout before rendering it.
  if (current.runTarget !== runTarget) setCurrent({ runTarget, state: emptySplitView });
  const state = current.runTarget === runTarget ? current.state : emptySplitView;
  const dispatch = useCallback((action: SplitViewAction) => {
    setCurrent((previous) => {
      const next = reduceSplitView(previous.state, action);
      return next === previous.state ? previous : { ...previous, state: next };
    });
  }, []);
  const selectSession = useCallback((sessionId: string | null, focus = true) => {
    dispatch({ type: "select", sessionId, focus });
  }, [dispatch]);

  const liveSessionIds = useMemo(() => new Set(sessions
    .filter((session) => session.state !== "exited" && session.exited === undefined)
    .map((session) => session.id)), [sessions]);
  useEffect(() => {
    const closed = new Set(paneSessionIds(state.root).filter((id) => !liveSessionIds.has(id)));
    if (closed.size > 0) dispatch({ type: "prune", sessionIds: closed });
  }, [liveSessionIds, state.root, dispatch]);

  const members = paneSessionIds(state.root);
  function canSplit(sessionId: string): boolean {
    return !compact && state.activeSessionId !== null
      && liveSessionIds.has(state.activeSessionId) && liveSessionIds.has(sessionId)
      && sessionId !== state.activeSessionId
      && (members.includes(sessionId) || members.length < maximumSplitViewPanes);
  }
  function split({ sourceSessionId, targetSessionId, edge }: { sourceSessionId: string; targetSessionId: string; edge: PaneEdge }) {
    if (compact || !liveSessionIds.has(sourceSessionId) || !liveSessionIds.has(targetSessionId)) return;
    dispatch({ type: "split", sourceSessionId, targetSessionId, edge });
  }
  return { state, compact, dispatch, selectSession, canSplit, split };
}

export type SplitViewController = ReturnType<typeof useSplitView>;

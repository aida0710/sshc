import { useCallback, useEffect, useMemo, useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import { paneSessionIds, type PaneEdge } from "./layout";
import { activeSplitView, emptySplitGroups, reduceSplitGroups, type SplitGroupsAction } from "./splitGroups";

export function useSplitView({ sessions, runTarget }: { sessions: TerminalSession[]; runTarget: string | null }) {
  const [current, setCurrent] = useState({ runTarget, state: emptySplitGroups });
  const compact = useMediaQuery(mobileViewportQuery);
  // A target change must discard the old engine's layout before rendering it.
  if (current.runTarget !== runTarget) setCurrent({ runTarget, state: emptySplitGroups });
  const groupState = current.runTarget === runTarget ? current.state : emptySplitGroups;
  const state = activeSplitView(groupState);
  const dispatch = useCallback((action: SplitGroupsAction) => {
    setCurrent((previous) => {
      const next = reduceSplitGroups(previous.state, action);
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
    const closed = new Set(groupState.groups.flatMap((group) => paneSessionIds(group.view.root)).filter((id) => !liveSessionIds.has(id)));
    if (closed.size > 0) dispatch({ type: "prune", sessionIds: closed });
  }, [liveSessionIds, groupState.groups, dispatch]);

  function canSplit(sessionId: string, targetSessionId = state.activeSessionId): boolean {
    return !compact && targetSessionId !== null
      && liveSessionIds.has(targetSessionId) && liveSessionIds.has(sessionId)
      && sessionId !== targetSessionId
      && reduceSplitGroups(groupState, { type: "split", sourceSessionId: sessionId, targetSessionId, edge: "right" }) !== groupState;
  }
  function split({ sourceSessionId, targetSessionId, edge }: { sourceSessionId: string; targetSessionId: string; edge: PaneEdge }) {
    if (compact || !liveSessionIds.has(sourceSessionId) || !liveSessionIds.has(targetSessionId)) return;
    dispatch({ type: "split", sourceSessionId, targetSessionId, edge });
  }
  const groupNumberBySession = new Map(groupState.groups.flatMap((group) => paneSessionIds(group.view.root).map((id) => [id, group.number] as const)));
  return { state, groups: groupState.groups, activeGroupNumber: groupState.activeGroupNumber, groupNumberBySession, compact, dispatch, selectSession, canSplit, split };
}

export type SplitViewController = ReturnType<typeof useSplitView>;

import { useCallback, useEffect, useMemo, useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import { paneSessionIds, type PaneEdge } from "./layout";
import { activeSplitView, emptySplitGroups, reduceSplitGroups, type SplitGroupsAction, type SplitGroupsState } from "./splitGroups";

// SplitRequest is a session waiting to be told what goes beside it, and on which side.
export type SplitRequest = { targetSessionId: string; edge: PaneEdge };

// SplitRefusal says why nothing can go beside a session: it has ended, every split a new pane would need is taken,
// or no other session can be opened.
export type SplitRefusal = "ended" | "groups" | "sessions";

// A session not open yet has no id. The reducer only needs one to ask whether a new pane has room.
const sessionNotOpenYet = "session-not-open-yet";

export function useSplitView({ sessions, sessionsLoaded = true, runTarget, maxSessions = 0 }: {
  sessions: TerminalSession[];
  // sessionsLoaded says whether `sessions` is the run target's own list. Until it is, the list is still the one
  // the previous run target answered with, or empty.
  sessionsLoaded?: boolean;
  runTarget: string | null;
  // maxSessions is how many live sessions the engine allows; 0 until it has said.
  maxSessions?: number;
}) {
  // Each run target keeps its own splits, so that coming back to one finds them as they were left.
  const [groupsByRunTarget, setGroupsByRunTarget] = useState<ReadonlyMap<string | null, SplitGroupsState>>(new Map());
  const [splitRequest, setSplitRequest] = useState<SplitRequest | null>(null);
  const [requestRunTarget, setRequestRunTarget] = useState(runTarget);
  const compact = useMediaQuery(mobileViewportQuery);
  // A request for a split names a session of the run target it was made on.
  if (requestRunTarget !== runTarget) {
    setRequestRunTarget(runTarget);
    setSplitRequest(null);
  }
  // Until the run target's list is read, its panes cannot be told from closed sessions, so none are shown and
  // none are pruned.
  const groupState = sessionsLoaded ? groupsByRunTarget.get(runTarget) ?? emptySplitGroups : emptySplitGroups;
  const state = activeSplitView(groupState);
  const dispatch = useCallback((action: SplitGroupsAction) => {
    setGroupsByRunTarget((previous) => {
      const before = previous.get(runTarget) ?? emptySplitGroups;
      const next = reduceSplitGroups(before, action);
      return next === before ? previous : new Map(previous).set(runTarget, next);
    });
  }, [runTarget]);
  const selectSession = useCallback((sessionId: string | null, focus = true) => {
    dispatch({ type: "select", sessionId, focus });
  }, [dispatch]);

  const liveSessionIds = useMemo(() => new Set(sessions
    .filter((session) => session.state !== "exited" && session.exited === undefined)
    .map((session) => session.id)), [sessions]);
  useEffect(() => {
    if (!sessionsLoaded) return;
    const closed = new Set(groupState.groups.flatMap((group) => paneSessionIds(group.view.root)).filter((id) => !liveSessionIds.has(id)));
    if (closed.size > 0) dispatch({ type: "prune", sessionIds: closed });
  }, [sessionsLoaded, liveSessionIds, groupState.groups, dispatch]);

  function canSplit(sessionId: string, targetSessionId = state.activeSessionId): boolean {
    return !compact && targetSessionId !== null
      && liveSessionIds.has(targetSessionId) && liveSessionIds.has(sessionId)
      && sessionId !== targetSessionId
      && reduceSplitGroups(groupState, { type: "split", sourceSessionId: sessionId, targetSessionId, edge: "right" }) !== groupState;
  }
  const atSessionLimit = maxSessions > 0 && liveSessionIds.size >= maxSessions;
  // canOpenBeside says whether a session not open yet could take a pane beside the target.
  function canOpenBeside(targetSessionId: string): boolean {
    return !compact && !atSessionLimit && liveSessionIds.has(targetSessionId)
      && reduceSplitGroups(groupState, { type: "split", sourceSessionId: sessionNotOpenYet, targetSessionId, edge: "right" }) !== groupState;
  }
  // splitRefusal is why neither a session already open nor a new one can go beside the target, or null when one can.
  function splitRefusal(targetSessionId: string): SplitRefusal | null {
    if (!liveSessionIds.has(targetSessionId)) return "ended";
    if (sessions.some((candidate) => canSplit(candidate.id, targetSessionId)) || canOpenBeside(targetSessionId)) return null;
    return atSessionLimit ? "sessions" : "groups";
  }
  const cancelSplitRequest = useCallback(() => setSplitRequest(null), []);
  function split({ sourceSessionId, targetSessionId, edge }: { sourceSessionId: string; targetSessionId: string; edge: PaneEdge }) {
    if (compact || !liveSessionIds.has(sourceSessionId) || !liveSessionIds.has(targetSessionId)) return;
    dispatch({ type: "split", sourceSessionId, targetSessionId, edge });
  }
  // removeFromSplit takes a session out of whichever split it is in, active or
  // not; the session goes on outside the split. It leaves the split the same
  // way a closed session does.
  const removeFromSplit = useCallback((sessionId: string) => {
    dispatch({ type: "prune", sessionIds: new Set([sessionId]) });
  }, [dispatch]);
  const groupBySession = new Map(groupState.groups.flatMap((group) => paneSessionIds(group.view.root).map((id) => [id, group] as const)));
  return {
    state, groups: groupState.groups, activeGroupNumber: groupState.activeGroupNumber, groupBySession, compact, dispatch, selectSession,
    canSplit, canOpenBeside, splitRefusal, split, removeFromSplit, splitRequest, requestSplit: setSplitRequest, cancelSplitRequest,
  };
}

export type SplitViewController = ReturnType<typeof useSplitView>;

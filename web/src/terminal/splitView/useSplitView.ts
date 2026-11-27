import { useCallback, useEffect, useMemo, useState } from "react";
import type { TerminalSession } from "../../api/terminalSessions";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import type { SplitArea } from "./geometry";
import { paneSessionIds, type PaneEdge } from "./layout";
import { activeSplitView, emptySplitGroups, reduceSplitGroups, type SplitGroupsAction, type SplitGroupsState } from "./splitGroups";
import { useSplitArea } from "./useSplitArea";

// SplitRequest is a session waiting to be told what goes beside it, and on which side.
export type SplitRequest = { targetSessionId: string; edge: PaneEdge };

// SplitRefusal says why nothing can go beside a session: it has ended, every split a new pane would need is taken,
// or no other session can be opened. room は、その側にペインを足すと、どれかのペインが最小の大きさを割ること。
export type SplitRefusal = "ended" | "groups" | "sessions" | "room";

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
  const { area, observeArea } = useSplitArea();
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

  // acceptsSplit は、分割表示が、sourceSessionId のペインを target の edge の側に並べる分割を受け入れるかを返す。
  // areaToFit を渡すと、その広さでどのペインにも最小の大きさが残るかも見る（null なら見ない）。
  function acceptsSplit({ sourceSessionId, targetSessionId, edge }: SplitRequest & { sourceSessionId: string }, areaToFit: SplitArea | null): boolean {
    return reduceSplitGroups(groupState, { type: "split", sourceSessionId, targetSessionId, edge, area: areaToFit ?? undefined }) !== groupState;
  }
  function canSplit(sessionId: string, targetSessionId: string | null, edge: PaneEdge): boolean {
    return !compact && targetSessionId !== null
      && liveSessionIds.has(targetSessionId) && liveSessionIds.has(sessionId)
      && sessionId !== targetSessionId
      && acceptsSplit({ sourceSessionId: sessionId, targetSessionId, edge }, area);
  }
  const atSessionLimit = maxSessions > 0 && liveSessionIds.size >= maxSessions;
  // canOpenBeside says whether a session not open yet could take a pane beside the target.
  function canOpenBeside(targetSessionId: string, edge: PaneEdge): boolean {
    return !compact && !atSessionLimit && liveSessionIds.has(targetSessionId)
      && acceptsSplit({ sourceSessionId: sessionNotOpenYet, targetSessionId, edge }, area);
  }
  // lacksRoomFor は、sourceSessionId のペイン（null なら新しいペイン）を target の edge の側に並べる分割を、
  // ペインの数や分割の数でなく、分割表示の広さだけが理由で断るかを返す。
  function lacksRoomFor(targetSessionId: string, edge: PaneEdge, sourceSessionId: string | null): boolean {
    const request = { sourceSessionId: sourceSessionId ?? sessionNotOpenYet, targetSessionId, edge };
    return acceptsSplit(request, null) && !acceptsSplit(request, area);
  }
  // splitRefusal is why neither a session already open nor a new one can go beside the target, or null when one can.
  // 開いているセッションか新しいセッションのどれかが、広さだけが理由で置けないなら room とする。4つ並んだ分割を
  // 狭めたときは、新しいペインはペインの数で、分割の中のペインの移動は広さで断るため。
  function splitRefusal(targetSessionId: string, edge: PaneEdge): SplitRefusal | null {
    if (!liveSessionIds.has(targetSessionId)) return "ended";
    const candidates = sessions.filter((candidate) => candidate.id !== targetSessionId && liveSessionIds.has(candidate.id));
    if (candidates.some((candidate) => canSplit(candidate.id, targetSessionId, edge)) || canOpenBeside(targetSessionId, edge)) return null;
    const refusedForRoom = [...candidates.map((candidate) => candidate.id), null].some((sourceSessionId) => lacksRoomFor(targetSessionId, edge, sourceSessionId));
    if (refusedForRoom) return "room";
    return atSessionLimit ? "sessions" : "groups";
  }
  const cancelSplitRequest = useCallback(() => setSplitRequest(null), []);
  function split({ sourceSessionId, targetSessionId, edge }: { sourceSessionId: string; targetSessionId: string; edge: PaneEdge }) {
    if (compact || !liveSessionIds.has(sourceSessionId) || !liveSessionIds.has(targetSessionId)) return;
    dispatch({ type: "split", sourceSessionId, targetSessionId, edge, area: area ?? undefined });
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
    canSplit, canOpenBeside, lacksRoomFor, splitRefusal, split, removeFromSplit, splitRequest, requestSplit: setSplitRequest, cancelSplitRequest,
    area, observeArea,
  };
}

export type SplitViewController = ReturnType<typeof useSplitView>;

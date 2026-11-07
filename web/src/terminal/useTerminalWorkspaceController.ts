import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { OpenTerminalSessionRequest } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import { sshSessionTarget } from "../connections/directDestination";
import type { Section } from "../routing/sectionRoute";
import { duplicateSessionRequest } from "./duplicateSessionRequest";
import { useSplitView, type SplitRequest } from "./splitView/useSplitView";
import type { TerminalSessionsState } from "./sessions";
import { useOpenAfterRemoteUnlock } from "./useOpenAfterRemoteUnlock";
import { topSessionId } from "./workspaceGrouping";

type TerminalWorkspaceControllerOptions = {
  terminalSessions: TerminalSessionsState;
  // workspaces are the run target's, in the sidebar's order, to know which
  // session is at the top of it.
  workspaces: readonly TerminalWorkspace[];
  runTarget?: string | null;
  section: Section | null;
  navigate: (section: Section) => void;
  closeNavigation: () => void;
  // vaultRevision counts the remote Vault unlocks, for a session refused while it was locked to open after one.
  vaultRevision?: number;
};

export function useTerminalWorkspaceController({
  terminalSessions,
  workspaces,
  runTarget = null,
  section,
  navigate,
  closeNavigation,
  vaultRevision = 0,
}: TerminalWorkspaceControllerOptions) {
  const splitView = useSplitView({ sessions: terminalSessions.sessions, runTarget, maxSessions: terminalSessions.maxSessions });
  const { selectSession: setActiveSessionId, dispatch: dispatchSplit } = splitView;
  const { activeSessionId } = splitView.state;
  const [sessionOrder, setSessionOrder] = useState<string[]>([]);
  const pendingSessionId = useRef<string | null>(null);
  // The session last shown on each run target (null is this machine), shown
  // again when the person comes back to it. A run target's selection starts
  // empty in useSplitView, so what is recorded here is always that target's.
  const lastSessionByRunTarget = useRef(new Map<string | null, string>());

  const orderedSessions = useMemo(() => {
    const rank = new Map(sessionOrder.map((id, index) => [id, index]));
    return terminalSessions.sessions
      .map((session, index) => ({
        session,
        rank: rank.get(session.id) ?? sessionOrder.length + index,
      }))
      .sort((left, right) => left.rank - right.rank)
      .map((entry) => entry.session);
  }, [terminalSessions.sessions, sessionOrder]);

  useEffect(() => {
    if (activeSessionId === null) return;
    // Split view removes ended or deleted members and selects a surviving pane.
    if (splitView.state.root !== null && !("sessionId" in splitView.state.root)) return;
    if (terminalSessions.sessions.some((session) => session.id === activeSessionId)) {
      pendingSessionId.current = null;
      return;
    }
    // 開いた直後は一覧の再取得がまだ届いていない。取得が終わるまで選択を保ち、
    // 一覧の先頭へ勝手に戻らないようにする。
    if (pendingSessionId.current === activeSessionId) return;
    setActiveSessionId(null);
  }, [terminalSessions.sessions, activeSessionId, setActiveSessionId, splitView.state.root]);

  useEffect(() => {
    if (activeSessionId !== null) lastSessionByRunTarget.current.set(runTarget, activeSessionId);
  }, [activeSessionId, runTarget]);

  // With no session shown (the run target changed, or the shown one is gone),
  // the one last shown on this run target comes back while it is listed, and
  // otherwise the top of the sidebar.
  useEffect(() => {
    if (activeSessionId !== null || orderedSessions.length === 0) return;
    const lastShown = lastSessionByRunTarget.current.get(runTarget);
    const lastShownIsListed = lastShown !== undefined && orderedSessions.some((session) => session.id === lastShown);
    const next = lastShownIsListed ? lastShown : topSessionId(workspaces, orderedSessions);
    if (next !== null) setActiveSessionId(next);
  }, [orderedSessions, workspaces, activeSessionId, runTarget, setActiveSessionId]);

  const showSession = useCallback(
    (id: string) => {
      pendingSessionId.current = id;
      setActiveSessionId(id);
      closeNavigation();
      navigate("Terminal");
      void terminalSessions.refresh().finally(() => {
        if (pendingSessionId.current === id) pendingSessionId.current = null;
      });
    },
    [closeNavigation, terminalSessions, navigate, setActiveSessionId],
  );

  // The terminal screen stays drawn behind other sections while it holds a
  // session.
  const terminalScreenMounted = section === "Terminal" || activeSessionId !== null;

  // openSession opens a session and shows it. On a run target, opening waits on
  // the SSH connection to that host, which takes until its timeout when the
  // host does not answer, so the terminal screen is shown at once: it says that
  // the host is being reached and, if it cannot be, why. On this machine a
  // failure stays on the current screen.
  const openAndShow = useCallback(
    async (request: OpenTerminalSessionRequest) => {
      const opened = await terminalSessions.open(request);
      if (opened !== null) showSession(opened.id);
      return opened;
    },
    [showSession, terminalSessions],
  );
  const { noteRefused } = useOpenAfterRemoteUnlock({
    runTarget, refusal: terminalSessions.remoteRefusal, vaultRevision, open: openAndShow,
  });

  const openSession = useCallback(
    async (request: OpenTerminalSessionRequest) => {
      if (runTarget !== null) {
        closeNavigation();
        navigate("Terminal");
      }
      const opened = await openAndShow(request);
      if (opened === null) noteRefused(request);
      return opened;
    },
    [closeNavigation, navigate, noteRefused, openAndShow, runTarget],
  );

  // workspaceId is where the session goes; without one the engine puts it in
  // the default workspace.
  const openLocalShell = useCallback(
    async (profileId?: string, cwd?: string, workspaceId?: string) => {
      await openSession({
        kind: "shell",
        ...(profileId === undefined ? {} : { profileId }),
        ...(cwd === undefined ? {} : { cwd }),
        ...(workspaceId === undefined ? {} : { workspaceId }),
      });
    },
    [openSession],
  );

  // cwd は、SFTP で開いているフォルダからターミナルを開くときの開始位置である。
  const openSSHSession = useCallback(
    async (alias: string, cwd?: string, workspaceId?: string) => {
      await openSession({
        kind: "ssh",
        ...sshSessionTarget(alias),
        ...(cwd === undefined ? {} : { cwd }),
        ...(workspaceId === undefined ? {} : { workspaceId }),
      });
    },
    [openSession],
  );

  const duplicateSession = useCallback(
    async (id: string) => {
      const session = terminalSessions.sessions.find(
        (candidate) => candidate.id === id,
      );
      if (session === undefined) return null;
      return openSession(duplicateSessionRequest(session));
    },
    [terminalSessions.sessions, openSession],
  );

  // openSessionInSplit opens a session in a new pane beside another. It does not show the session first: selecting
  // one outside the split would put it in place of the panes, and the split would lose the session it goes beside.
  // The list already holds the new session in the render this split lands in (terminalSessions.open), so the split
  // does not take it for one that has ended.
  const openSessionInSplit = useCallback(
    async (request: OpenTerminalSessionRequest, place: SplitRequest) => {
      const opened = await terminalSessions.open(request);
      if (opened === null) return;
      dispatchSplit({ type: "split", sourceSessionId: opened.id, ...place });
      closeNavigation();
      navigate("Terminal");
    },
    [closeNavigation, dispatchSplit, navigate, terminalSessions],
  );

  return {
    splitView,
    activeSessionId,
    terminalScreenMounted,
    orderedSessions,
    showSession,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    openSessionInSplit,
    reorderSessions: setSessionOrder,
  };
}

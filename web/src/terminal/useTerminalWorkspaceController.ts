import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LocalShellProfileList, SettingsApi, TerminalSettings } from "../api/settings";
import type { OpenTerminalSessionRequest, TerminalSessionsApi } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import type { Section } from "../routing/sectionRoute";
import { useSplitView } from "./splitView/useSplitView";
import type { TerminalSessionsState } from "./sessions";
import { topSessionId } from "./workspaceGrouping";

type TerminalWorkspaceControllerOptions = {
  api: Pick<SettingsApi, "terminalSettings">;
  // shells is the terminal API of the engine in use: its shells are the ones
  // a new session can open, on this machine or on the run target.
  shells: Pick<TerminalSessionsApi, "shellProfiles">;
  terminalSessions: TerminalSessionsState;
  // workspaces are the run target's, in the sidebar's order, to know which
  // session is at the top of it.
  workspaces: readonly TerminalWorkspace[];
  enabled: boolean;
  runTarget?: string | null;
  section: Section | null;
  navigate: (section: Section) => void;
  closeNavigation: () => void;
};

export function useTerminalWorkspaceController({
  api,
  shells,
  terminalSessions,
  workspaces,
  enabled,
  runTarget = null,
  section,
  navigate,
  closeNavigation,
}: TerminalWorkspaceControllerOptions) {
  const [settings, setSettings] = useState<TerminalSettings>({});
  const [localShellProfiles, setLocalShellProfiles] = useState<LocalShellProfileList["profiles"]>([]);
  const splitView = useSplitView({ sessions: terminalSessions.sessions, runTarget });
  const { selectSession: setActiveSessionId } = splitView;
  const { activeSessionId } = splitView.state;
  const [sessionOrder, setSessionOrder] = useState<string[]>([]);
  const pendingSessionId = useRef<string | null>(null);
  // The session last shown on each run target (null is this machine), shown
  // again when the person comes back to it. A run target's selection starts
  // empty in useSplitView, so what is recorded here is always that target's.
  const lastSessionByRunTarget = useRef(new Map<string | null, string>());

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void api
      .terminalSettings()
      .then((next) => {
        if (active) setSettings(next);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [api, enabled, section]);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    // The shells of the previous engine are not offered while the next one's
    // are read.
    setLocalShellProfiles([]);
    void shells
      .shellProfiles()
      .then((answer) => {
        if (active) setLocalShellProfiles(answer.profiles);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [shells, enabled, section]);

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
  const openSession = useCallback(
    async (request: OpenTerminalSessionRequest) => {
      if (runTarget !== null) {
        closeNavigation();
        navigate("Terminal");
      }
      const opened = await terminalSessions.open(request);
      if (opened !== null) showSession(opened.id);
      return opened;
    },
    [closeNavigation, navigate, runTarget, showSession, terminalSessions],
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
        alias,
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
      // A duplicate opens next to the original, in the same workspace and the
      // directory the original works in, when the engine knows it.
      const startIn = session.workingDirectory === undefined ? {} : { cwd: session.workingDirectory };
      return openSession(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias, workspaceId: session.workspaceId, ...startIn }
          : {
              kind: "shell", workspaceId: session.workspaceId, ...startIn,
              ...(session.shellProfileId === undefined ? {} : { profileId: session.shellProfileId }),
            },
      );
    },
    [terminalSessions.sessions, openSession],
  );

  return {
    splitView,
    settings,
    localShellProfiles,
    activeSessionId,
    terminalScreenMounted,
    orderedSessions,
    showSession,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    reorderSessions: setSessionOrder,
    setSettings,
  };
}

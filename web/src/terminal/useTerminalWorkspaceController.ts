import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LocalShellProfileList, SettingsApi, TerminalSettings } from "../api/settings";
import type { Section } from "../routing/sectionRoute";
import type { WorkspaceRenameRequest, WorkspaceRestoreRequest } from "../features/workspaces/TerminalWorkspace";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import type { TerminalSessionsState } from "./sessions";

type TerminalWorkspaceControllerOptions = {
  api: Pick<SettingsApi, "terminalSettings" | "localShellProfiles">;
  terminalSessions: TerminalSessionsState;
  enabled: boolean;
  section: Section | null;
  navigate: (section: Section) => void;
  closeNavigation: () => void;
};

export function useTerminalWorkspaceController({
  api,
  terminalSessions,
  enabled,
  section,
  navigate,
  closeNavigation,
}: TerminalWorkspaceControllerOptions) {
  const [settings, setSettings] = useState<TerminalSettings>({});
  const [localShellProfiles, setLocalShellProfiles] = useState<LocalShellProfileList["profiles"]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [liveWorkspace, setLiveWorkspace] =
    useState<LiveWorkspaceSummary | null>(null);
  const [restoreRequest, setRestoreRequest] =
    useState<WorkspaceRestoreRequest | null>(null);
  const [renameRequest, setRenameRequest] =
    useState<WorkspaceRenameRequest | null>(null);
  const [workspaceRestoring, setWorkspaceRestoring] = useState(false);
  const [sessionOrder, setSessionOrder] = useState<string[]>([]);
  const restoreSequence = useRef(0);
  const renameSequence = useRef(0);
  const pendingSessionId = useRef<string | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void api
      .terminalSettings()
      .then((next) => {
        if (active) setSettings(next);
      })
      .catch(() => undefined);
    if (api.localShellProfiles !== undefined) {
      void api
        .localShellProfiles()
        .then((answer) => {
          if (active) setLocalShellProfiles(answer.profiles);
        })
        .catch(() => undefined);
    }
    return () => {
      active = false;
    };
  }, [api, enabled, section]);

  useEffect(() => {
    if (activeSessionId === null) return;
    if (terminalSessions.sessions.some((session) => session.id === activeSessionId)) {
      pendingSessionId.current = null;
      return;
    }
    // 開いた直後は一覧の再取得がまだ届いていない。取得が終わるまで選択を保ち、
    // 一覧の先頭へ勝手に戻らないようにする。
    if (pendingSessionId.current === activeSessionId) return;
    setActiveSessionId(null);
  }, [terminalSessions.sessions, activeSessionId]);

  useEffect(() => {
    if (activeSessionId !== null || terminalSessions.sessions.length === 0) return;
    setActiveSessionId(terminalSessions.sessions[0]?.id ?? null);
  }, [terminalSessions.sessions, activeSessionId]);

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
    [closeNavigation, terminalSessions, navigate],
  );

  const openWorkspace = useCallback(
    (id: string) => {
      restoreSequence.current += 1;
      setRestoreRequest({ id, sequence: restoreSequence.current });
      navigate("Terminal");
    },
    [navigate],
  );

  const consumeRestore = useCallback((sequence: number) => {
    setRestoreRequest((current) =>
      current?.sequence === sequence ? null : current,
    );
  }, []);

  const renameWorkspace = useCallback(
    (name: string) => {
      renameSequence.current += 1;
      setRenameRequest({ name, sequence: renameSequence.current });
      closeNavigation();
      navigate("Terminal");
    },
    [closeNavigation, navigate],
  );

  const consumeRename = useCallback((sequence: number) => {
    setRenameRequest((current) =>
      current?.sequence === sequence ? null : current,
    );
  }, []);

  // The terminal screen stays drawn behind other sections while it holds a
  // session. A workspace restore counts too: its first session may not exist
  // yet, and removing the screen would abandon the restore and close every
  // session it has opened.
  const terminalScreenMounted =
    section === "Terminal" || activeSessionId !== null || workspaceRestoring;

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

  const openLocalShell = useCallback(
    async (profileId?: string, cwd?: string) => {
      const opened = await terminalSessions.open({
        kind: "shell",
        ...(profileId === undefined ? {} : { profileId }),
        ...(cwd === undefined ? {} : { cwd }),
      });
      if (opened !== null) showSession(opened.id);
    },
    [terminalSessions, showSession],
  );

  // cwd は、SFTP で開いているフォルダからターミナルを開くときの開始位置である。
  const openSSHSession = useCallback(
    async (alias: string, cwd?: string) => {
      const opened = await terminalSessions.open({
        kind: "ssh",
        alias,
        ...(cwd === undefined ? {} : { cwd }),
      });
      if (opened !== null) showSession(opened.id);
    },
    [terminalSessions, showSession],
  );

  const duplicateSession = useCallback(
    async (id: string) => {
      const session = terminalSessions.sessions.find(
        (candidate) => candidate.id === id,
      );
      if (session === undefined) return null;
      const opened = await terminalSessions.open(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias }
          : { kind: "shell" },
      );
      if (opened !== null) showSession(opened.id);
      return opened;
    },
    [terminalSessions, showSession],
  );

  return {
    settings,
    localShellProfiles,
    activeSessionId,
    terminalScreenMounted,
    liveWorkspace,
    restoreRequest,
    renameRequest,
    orderedSessions,
    showSession,
    openWorkspace,
    renameWorkspace,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    consumeRestore,
    setWorkspaceRestoring,
    consumeRename,
    reorderSessions: setSessionOrder,
    setLiveWorkspace,
    setSettings,
  };
}

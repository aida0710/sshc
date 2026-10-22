import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { LocalShellProfileList, SettingsApi, TerminalSettings } from "../api/settings";
import type { Section } from "../routing/sectionRoute";
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
  const [sessionOrder, setSessionOrder] = useState<string[]>([]);
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

  // The terminal screen stays drawn behind other sections while it holds a
  // session.
  const terminalScreenMounted = section === "Terminal" || activeSessionId !== null;

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

  // workspaceId is where the session goes; without one the engine puts it in
  // the default workspace.
  const openLocalShell = useCallback(
    async (profileId?: string, cwd?: string, workspaceId?: string) => {
      const opened = await terminalSessions.open({
        kind: "shell",
        ...(profileId === undefined ? {} : { profileId }),
        ...(cwd === undefined ? {} : { cwd }),
        ...(workspaceId === undefined ? {} : { workspaceId }),
      });
      if (opened !== null) showSession(opened.id);
    },
    [terminalSessions, showSession],
  );

  // cwd は、SFTP で開いているフォルダからターミナルを開くときの開始位置である。
  const openSSHSession = useCallback(
    async (alias: string, cwd?: string, workspaceId?: string) => {
      const opened = await terminalSessions.open({
        kind: "ssh",
        alias,
        ...(cwd === undefined ? {} : { cwd }),
        ...(workspaceId === undefined ? {} : { workspaceId }),
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
      // A duplicate opens next to the original, in the same workspace.
      const opened = await terminalSessions.open(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias, workspaceId: session.workspaceId }
          : { kind: "shell", workspaceId: session.workspaceId },
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
    orderedSessions,
    showSession,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    reorderSessions: setSessionOrder,
    setSettings,
  };
}

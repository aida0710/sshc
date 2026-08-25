import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import {
  type IntegrationsApi,
  type OpenTerminalSessionRequest,
  type TerminalSession,
} from "../api/integrations";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

export type TerminalSessionsApi = Pick<
  IntegrationsApi,
  "terminalSessions" | "openTerminalSession" | "reconnectTerminalSession" | "closeTerminalSession" | "renameTerminalSession"
>;

export type TerminalSessionsState = {
  sessions: TerminalSession[];
  maxSessions: number;
  busy: boolean;
  problem: string;
  loaded: boolean;
  rename: (id: string, title: string) => Promise<boolean>;
  open: (request: OpenTerminalSessionRequest) => Promise<TerminalSession | null>;
  reconnect: (id: string) => Promise<boolean>;
  close: (id: string) => Promise<void>;
  closeAll: () => Promise<void>;
  refresh: () => Promise<void>;
  markExited: (id: string) => void;
};

const closeAllRounds = 10;
const closeAllPause = 100;

export function useTerminalSessions(
  api: TerminalSessionsApi,
  translate: Translate,
  enabled = true,
): TerminalSessionsState {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [maxSessions, setMaxSessions] = useState(0);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [loaded, setLoaded] = useState(false);
  const refreshGeneration = useRef(0);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    const generation = refreshGeneration.current + 1;
    refreshGeneration.current = generation;
    try {
      const listed = await api.terminalSessions();
      if (generation !== refreshGeneration.current) return;
      setSessions(listed.sessions);
      setMaxSessions(listed.maxSessions);
    } catch {
    } finally {
      if (generation === refreshGeneration.current) setLoaded(true);
    }
  }, [api, enabled]);

  useEffect(() => {
    void refresh();
    return () => {
      refreshGeneration.current += 1;
    };
  }, [refresh]);

  useEffect(() => {
    if (!enabled || sessions.length === 0) return;
    const timer = window.setInterval(() => {
      if (!document.hidden) void refresh();
    }, 2_000);
    return () => window.clearInterval(timer);
  }, [enabled, refresh, sessions.length]);

  const open = useCallback(
    async (request: OpenTerminalSessionRequest): Promise<TerminalSession | null> => {
      setBusy(true);
      setProblem("");
      try {
        const opened = await api.openTerminalSession(request);
        await refresh();
        return opened.session;
      } catch (error) {
        setProblem(translate(terminalProblemKey(failureCode(error))));
        return null;
      } finally {
        setBusy(false);
      }
    },
    [api, refresh, translate],
  );

  const close = useCallback(
    async (id: string) => {
      setBusy(true);
      try {
        const listed = await api.closeTerminalSession(id);
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
      } catch {
        setProblem(translate("terminal.closeFailed"));
      } finally {
        setBusy(false);
      }
    },
    [api, translate],
  );

  const reconnect = useCallback(
    async (id: string): Promise<boolean> => {
      setBusy(true);
      setProblem("");
      try {
        const listed = await api.reconnectTerminalSession(id);
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
        return true;
      } catch (error) {
        setProblem(translate(terminalProblemKey(failureCode(error))));
        await refresh();
        return false;
      } finally {
        setBusy(false);
      }
    },
    [api, refresh, translate],
  );

  const closeAll = useCallback(async () => {
    setBusy(true);
    let failed = false;
    try {
      let remaining = sessions;
      for (let round = 0; round < closeAllRounds && remaining.length > 0; round += 1) {
        if (round > 0) await new Promise((resume) => setTimeout(resume, closeAllPause));
        for (const session of remaining) {
          try {
            const listed = await api.closeTerminalSession(session.id);
            setSessions(listed.sessions);
            setMaxSessions(listed.maxSessions);
            remaining = listed.sessions;
          } catch {
            failed = true;
          }
        }
        const listed = await api.terminalSessions().catch(() => null);
        if (listed === null) break;
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
        remaining = listed.sessions;
      }
      if (failed && remaining.length > 0) setProblem(translate("terminal.closeFailed"));
    } finally {
      setBusy(false);
    }
  }, [api, sessions, translate]);

  const rename = useCallback(
    async (id: string, title: string): Promise<boolean> => {
      try {
        const listed = await api.renameTerminalSession(id, title);
        setSessions(listed.sessions);
        setMaxSessions(listed.maxSessions);
        return true;
      } catch {
        setProblem(translate("terminal.renameFailed"));
        return false;
      }
    },
    [api, translate],
  );

  const markExited = useCallback((id: string) => {
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, state: "exited", exited: { code: 0, signal: "", at: "" } }
          : session,
      ),
    );
  }, []);

  return { sessions, maxSessions, busy, problem, loaded, rename, open, reconnect, close, closeAll, refresh, markExited };
}

export function terminalProblemKey(code: string): MessageKey {
  switch (code) {
    case "terminal_session_limit":
      return "terminal.limitRefused";
    case "alias_unresolvable":
      return "terminal.unresolvable";
    case "proxy_command_with_jump":
      return "terminal.proxyCommandWithJump";
    case "jump_depth_exceeded":
      return "terminal.jumpDepthExceeded";
    case "host_key_unknown":
      return "terminal.hostKeyUnknown";
    case "host_key_changed":
      return "terminal.hostKeyChanged";
    case "host_key_revoked":
      return "terminal.hostKeyRevoked";
    case "identity_unavailable":
      return "terminal.identityUnavailable";
    case "authentication_unavailable":
      return "terminal.authenticationUnavailable";
    case "authentication_cancelled":
      return "terminal.authenticationCancelled";
    case "key_passphrase_required":
      return "terminal.keyPassphraseRequired";
    case "reconnect_failed":
      return "terminal.reconnectFailed";
    case "reconnect_exhausted":
      return "terminal.reconnectExhausted";
    default:
      return "terminal.openFailed";
  }
}

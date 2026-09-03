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
  | "resumeTerminalAgent"
>;

export type TerminalSessionsState = {
  sessions: TerminalSession[];
  maxSessions: number;
  busy: boolean;
  problem: string;
  loaded: boolean;
  rename: (id: string, title: string) => Promise<boolean>;
  unpinTitle?: (id: string) => Promise<boolean>;
  open: (request: OpenTerminalSessionRequest) => Promise<TerminalSession | null>;
  reconnect: (id: string) => Promise<boolean>;
  resumeAgent?: (id: string, observationVersion: number, placement: "same-pane" | "new-pane") => Promise<TerminalSession | null>;
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
  const [pendingOperations, setPendingOperations] = useState(0);
  const [problem, setProblem] = useState("");
  const [loaded, setLoaded] = useState(false);
  const refreshGeneration = useRef(0);
  const mutationGeneration = useRef(0);
  const busy = pendingOperations > 0;

  const beginOperation = useCallback(() => {
    setPendingOperations((current) => current + 1);
  }, []);

  const finishOperation = useCallback(() => {
    setPendingOperations((current) => Math.max(0, current - 1));
  }, []);

  const beginMutation = useCallback(() => {
    // A response from a list request which began before this mutation cannot
    // describe its result.
    refreshGeneration.current += 1;
    mutationGeneration.current += 1;
    return mutationGeneration.current;
  }, []);

  const adoptMutationListing = useCallback((
    listed: { sessions: TerminalSession[]; maxSessions: number },
    generation: number,
  ): boolean => {
    if (generation !== mutationGeneration.current) return false;
    // A mutation response describes state after the requested change. Retire
    // every list request that started before this response so a slow poll
    // cannot resurrect a closed session or restore an old title/state.
    refreshGeneration.current += 1;
    setSessions(listed.sessions);
    setMaxSessions(listed.maxSessions);
    return true;
  }, []);

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

  const connectionInProgress = sessions.some(
    (session) => session.state === "connecting" || session.state === "reconnecting",
  );

  useEffect(() => {
    if (!enabled || sessions.length === 0) return;
    // 接続中だけ細かく確認する。通常稼働中の一覧は従来どおり低頻度に保ち、
    // ProxyJumpのホップや認証待ちだけを人が追える速さで更新する。
    // Agent signals must still be observed while the app is in the background;
    // that is precisely when notification delivery is useful. Keep the normal
    // low-frequency poll hidden, while connection progress remains responsive.
    const timer = window.setInterval(() => void refresh(), connectionInProgress ? 500 : 2_000);
    return () => window.clearInterval(timer);
  }, [connectionInProgress, enabled, refresh, sessions.length]);

  const open = useCallback(
    async (request: OpenTerminalSessionRequest): Promise<TerminalSession | null> => {
      beginOperation();
      beginMutation();
      setProblem("");
      try {
        const opened = await api.openTerminalSession(request);
        await refresh();
        return opened.session;
      } catch (error) {
        setProblem(translate(terminalProblemKey(failureCode(error))));
        return null;
      } finally {
        finishOperation();
      }
    },
    [api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const close = useCallback(
    async (id: string) => {
      beginOperation();
      const generation = beginMutation();
      try {
        const listed = await api.closeTerminalSession(id);
        if (!adoptMutationListing(listed, generation)) await refresh();
      } catch {
        setProblem(translate("terminal.closeFailed"));
      } finally {
        finishOperation();
      }
    },
    [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const reconnect = useCallback(
    async (id: string): Promise<boolean> => {
      beginOperation();
      setProblem("");
      const generation = beginMutation();
      try {
        const listed = await api.reconnectTerminalSession(id);
        if (!adoptMutationListing(listed, generation)) await refresh();
        return true;
      } catch (error) {
        setProblem(translate(terminalProblemKey(failureCode(error))));
        await refresh();
        return false;
      } finally {
        finishOperation();
      }
    },
    [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const resumeAgent = useCallback(
    async (id: string, observationVersion: number, placement: "same-pane" | "new-pane"): Promise<TerminalSession | null> => {
      beginOperation();
      setProblem("");
      try {
        if (api.resumeTerminalAgent === undefined) return null;
        beginMutation();
        const resumed = await api.resumeTerminalAgent(id, { observationVersion, placement });
        await refresh();
        return resumed.session;
      } catch (error) {
        setProblem(translate(terminalProblemKey(failureCode(error))));
        await refresh();
        return null;
      } finally {
        finishOperation();
      }
    },
    [api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const closeAll = useCallback(async () => {
    beginOperation();
    let failed = false;
    try {
      let remaining = sessions;
      for (let round = 0; round < closeAllRounds && remaining.length > 0; round += 1) {
        if (round > 0) await new Promise((resume) => setTimeout(resume, closeAllPause));
        for (const session of remaining) {
          try {
            const generation = beginMutation();
            const listed = await api.closeTerminalSession(session.id);
            if (!adoptMutationListing(listed, generation)) await refresh();
            remaining = listed.sessions;
          } catch {
            failed = true;
          }
        }
        const generation = beginMutation();
        const listed = await api.terminalSessions().catch(() => null);
        if (listed === null) break;
        if (!adoptMutationListing(listed, generation)) await refresh();
        remaining = listed.sessions;
      }
      if (failed && remaining.length > 0) setProblem(translate("terminal.closeFailed"));
    } finally {
      finishOperation();
    }
  }, [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, sessions, translate]);

  const rename = useCallback(
    async (id: string, title: string): Promise<boolean> => {
      beginOperation();
      const generation = beginMutation();
      try {
        const listed = await api.renameTerminalSession(id, title);
        if (!adoptMutationListing(listed, generation)) await refresh();
        return true;
      } catch {
        setProblem(translate("terminal.renameFailed"));
        return false;
      } finally {
        finishOperation();
      }
    },
    [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const unpinTitle = useCallback(
    async (id: string): Promise<boolean> => {
      beginOperation();
      const generation = beginMutation();
      try {
        const listed = await api.renameTerminalSession(id, null);
        if (!adoptMutationListing(listed, generation)) await refresh();
        return true;
      } catch {
        setProblem(translate("terminal.renameFailed"));
        return false;
      } finally {
        finishOperation();
      }
    },
    [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, translate],
  );

  const markExited = useCallback((id: string) => {
    refreshGeneration.current += 1;
    mutationGeneration.current += 1;
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, state: "exited", exited: { code: 0, signal: "", at: "" } }
          : session,
      ),
    );
  }, []);

  return { sessions, maxSessions, busy, problem, loaded, rename, unpinTitle, open, reconnect, resumeAgent, close, closeAll, refresh, markExited };
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
    case "agent_resume_stale":
      return "terminal.agentResumeStale";
    case "agent_resume_same_pane_busy":
      return "terminal.agentResumeSamePaneBusy";
    case "agent_resume_unavailable":
      return "terminal.agentResumeUnavailable";
    case "agent_resume_identity_changed":
      return "terminal.agentResumeIdentityChanged";
    default:
      return "terminal.openFailed";
  }
}

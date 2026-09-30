import { useCallback, useEffect, useState } from "react";
import { usePolling } from "../ui/usePolling";
import { useRequestGeneration, type IsCurrentRequest } from "../ui/useRequestGeneration";
import { failureCode } from "../api/client";
import type { OpenTerminalSessionRequest, TerminalSession, TerminalSessionsApi as SessionsApi } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// A hop or an authentication prompt should appear as soon as a person can
// notice it; a settled list only needs to catch exits and notifications.
const connectingPollIntervalMs = 500;
const sessionListPollIntervalMs = 2_000;

export type TerminalSessionsApi = Pick<
  SessionsApi,
  "terminalSessions" | "openTerminalSession" | "reconnectTerminalSession" | "closeTerminalSession" | "setTerminalSessionTitle"
  | "stopTerminalReconnect"
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
  stopReconnect: (id: string) => Promise<boolean>;
  close: (id: string) => Promise<void>;
  closeAll: () => Promise<void>;
  refresh: () => Promise<void>;
  markExited: (id: string) => void;
};

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
  const refreshGeneration = useRequestGeneration();
  const mutationGeneration = useRequestGeneration();
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
    refreshGeneration.retire();
    return mutationGeneration.begin();
  }, [mutationGeneration, refreshGeneration]);

  const adoptMutationListing = useCallback((
    listed: { sessions: TerminalSession[]; maxSessions: number },
    isCurrent: IsCurrentRequest,
  ): boolean => {
    if (!isCurrent()) return false;
    // A mutation response describes state after the requested change. Retire
    // every list request that started before this response so a slow poll
    // cannot resurrect a closed session or restore an old title/state.
    refreshGeneration.retire();
    setSessions(listed.sessions);
    setMaxSessions(listed.maxSessions);
    return true;
  }, [refreshGeneration]);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    const isCurrent = refreshGeneration.begin();
    try {
      const listed = await api.terminalSessions();
      if (!isCurrent()) return;
      setSessions(listed.sessions);
      setMaxSessions(listed.maxSessions);
    } catch {
    } finally {
      if (isCurrent()) setLoaded(true);
    }
  }, [api, enabled, refreshGeneration]);

  // applyMutation runs one request whose response is the listing after the
  // change. A response that lost the race to a newer mutation is discarded in
  // favour of a fresh list. Callers only say what to show when it fails.
  const applyMutation = useCallback(async (
    request: () => Promise<{ sessions: TerminalSession[]; maxSessions: number }>,
    reportFailure: (error: unknown) => Promise<void> | void,
  ): Promise<boolean> => {
    beginOperation();
    const isCurrent = beginMutation();
    try {
      const listed = await request();
      if (!adoptMutationListing(listed, isCurrent)) await refresh();
      return true;
    } catch (error) {
      await reportFailure(error);
      return false;
    } finally {
      finishOperation();
    }
  }, [adoptMutationListing, beginMutation, beginOperation, finishOperation, refresh]);

  // Connection problems carry a fixed code the catalogue can translate; the
  // list is refreshed as well because the session state changed underneath.
  const reportConnectionFailure = useCallback(async (error: unknown) => {
    setProblem(translate(terminalProblemKey(failureCode(error))));
    await refresh();
  }, [refresh, translate]);

  useEffect(() => {
    void refresh();
    return () => {
      refreshGeneration.retire();
    };
  }, [refresh, refreshGeneration]);

  const connectionInProgress = sessions.some(
    (session) => session.state === "connecting" || session.state === "reconnecting",
  );

  // Only while a session is connecting or reconnecting does the list switch to
  // the faster interval, for the reason given at connectingPollIntervalMs.
  // Terminal notifications must still be observed while the app is in the
  // background; that is precisely when delivering them is useful.
  usePolling(refresh, {
    intervalMs: connectionInProgress ? connectingPollIntervalMs : sessionListPollIntervalMs,
    enabled: enabled && sessions.length > 0,
    whileHidden: true,
  });

  const open = useCallback(
    async (request: OpenTerminalSessionRequest): Promise<TerminalSession | null> => {
      beginOperation();
      setProblem("");
      try {
        const opened = await api.openTerminalSession(request);
        // Any listing requested before the engine created this session cannot
        // contain it. Retire those and show the session now, so a workspace
        // pane given its ID never finds it missing from the list.
        beginMutation();
        setSessions((current) =>
          current.some((session) => session.id === opened.session.id) ? current : [...current, opened.session],
        );
        // Hand the session back without waiting for the list to be read again,
        // so a caller that selects it does so in the render that lists it. A
        // wait here would show the new row for one round trip while the old
        // session is still selected, and a session shortcut pressed then would
        // move from the old one.
        void refresh();
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
      await applyMutation(() => api.closeTerminalSession(id), () => setProblem(translate("terminal.closeFailed")));
    },
    [api, applyMutation, translate],
  );

  const reconnect = useCallback(
    async (id: string): Promise<boolean> => {
      setProblem("");
      return applyMutation(() => api.reconnectTerminalSession(id), reportConnectionFailure);
    },
    [api, applyMutation, reportConnectionFailure],
  );

  const stopReconnect = useCallback(
    async (id: string): Promise<boolean> => {
      setProblem("");
      return applyMutation(() => api.stopTerminalReconnect(id), reportConnectionFailure);
    },
    [api, applyMutation, reportConnectionFailure],
  );

  const closeAll = useCallback(async () => {
    beginOperation();
    try {
      // The engine stops a session and removes it from the list before its
      // close answers, so one close per session is enough.
      const refusedIds = new Set<string>();
      for (const session of sessions) {
        try {
          const isCurrent = beginMutation();
          const listed = await api.closeTerminalSession(session.id);
          if (!adoptMutationListing(listed, isCurrent)) await refresh();
        } catch {
          refusedIds.add(session.id);
        }
      }
      if (refusedIds.size === 0) return;
      // A close is also refused for a session that already went away, so only
      // one still listed afterwards was left open.
      const isCurrent = beginMutation();
      const listed = await api.terminalSessions().catch(() => null);
      if (listed !== null && !adoptMutationListing(listed, isCurrent)) await refresh();
      const leftOpen = listed === null || listed.sessions.some((session) => refusedIds.has(session.id));
      if (leftOpen) setProblem(translate("terminal.closeFailed"));
    } finally {
      finishOperation();
    }
  }, [adoptMutationListing, api, beginMutation, beginOperation, finishOperation, refresh, sessions, translate]);

  const rename = useCallback(
    (id: string, title: string): Promise<boolean> =>
      applyMutation(() => api.setTerminalSessionTitle(id, title), () => setProblem(translate("terminal.renameFailed"))),
    [api, applyMutation, translate],
  );

  const unpinTitle = useCallback(
    (id: string): Promise<boolean> =>
      applyMutation(() => api.setTerminalSessionTitle(id, null), () => setProblem(translate("terminal.renameFailed"))),
    [api, applyMutation, translate],
  );

  const markExited = useCallback((id: string) => {
    beginMutation();
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, state: "exited", exited: { code: 0, signal: "", at: "" } }
          : session,
      ),
    );
  }, [beginMutation]);

  return { sessions, maxSessions, busy, problem, loaded, rename, unpinTitle, open, reconnect, stopReconnect, close, closeAll, refresh, markExited };
}

export function terminalProblemKey(code: string): MessageKey {
  switch (code) {
    case "terminal_session_limit":
      return "terminal.limitRefused";
    case "alias_unresolvable":
      return "terminal.unresolvable";
    case "remote_working_directory_unsupported":
      return "terminal.remoteWorkingDirectoryUnsupported";
    case "jump_depth_exceeded":
      return "terminal.jumpDepthExceeded";
    case "host_key_unknown":
      return "terminal.hostKeyUnknown";
    case "host_key_changed":
      return "terminal.hostKeyChanged";
    case "host_key_revoked":
      return "terminal.hostKeyRevoked";
    case "known_hosts_symlink":
      return "terminal.knownHostsSymlink";
    case "identity_unavailable":
      return "terminal.identityUnavailable";
    case "authentication_unavailable":
      return "terminal.authenticationUnavailable";
    case "proxy_authentication_required":
      return "terminal.proxyAuthenticationRequired";
    case "authentication_cancelled":
      return "terminal.authenticationCancelled";
    case "authentication_rejected":
      return "terminal.authenticationRejected";
    case "route_misconfigured":
      return "terminal.routeMisconfigured";
    case "key_passphrase_required":
      return "terminal.keyPassphraseRequired";
    case "vpn_route_refused":
      return "terminal.vpnRouteRefused";
    case "vpn_route_disconnected":
      return "terminal.vpnRouteDisconnected";
    case "connect_failed":
      return "terminal.connectFailed";
    case "reconnect_failed":
      return "terminal.reconnectFailed";
    case "reconnect_exhausted":
      return "terminal.reconnectExhausted";
    case "reconnect_stopped":
      return "terminal.reconnectStopped";
    case "terminal_not_reconnecting":
      return "terminal.notReconnecting";
    default:
      return "terminal.openFailed";
  }
}

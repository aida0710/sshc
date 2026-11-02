import { useCallback, useEffect, useState } from "react";
import { usePolling } from "../ui/usePolling";
import { useRequestGeneration, type IsCurrentRequest } from "../ui/useRequestGeneration";
import { ApiError, failureCode } from "../api/client";
import type { OpenTerminalSessionRequest, TerminalSession, TerminalSessionList, TerminalSessionsApi as SessionsApi } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { ExitReport } from "./stream";

// A hop or an authentication prompt should appear as soon as a person can
// notice it; a settled list only needs to catch exits and notifications.
const connectingPollIntervalMs = 500;
export const sessionListPollIntervalMs = 2_000;

export type TerminalSessionsApi = Pick<
  SessionsApi,
  "terminalSessions" | "openTerminalSession" | "reconnectTerminalSession" | "closeTerminalSession" | "setTerminalSessionTitle"
  | "stopTerminalReconnect" | "answerTerminalHostKey" | "moveTerminalSession"
>;

export type TerminalSessionsState = {
  sessions: TerminalSession[];
  maxSessions: number;
  busy: boolean;
  problem: string;
  // remoteRefusal is why the run target's engine refused to list its sessions;
  // null while it lists them. remoteRefusalAt is when it last refused.
  remoteRefusal: ApiError | null;
  remoteRefusalAt: Date | null;
  // opening is true while a session is being opened. On a run target that may
  // last as long as the SSH connection to it takes to fail.
  opening: boolean;
  // loaded is true once the engine in use has answered a listing (or a
  // change), whether it listed its sessions or refused.
  loaded: boolean;
  // restoreOffer names the engine's offer to reopen the sessions from before
  // it last stopped, while that waits for an answer. A new name means a new
  // engine: one that restarted while this screen kept listing it.
  restoreOffer: string | undefined;
  rename: (id: string, title: string) => Promise<boolean>;
  unpinTitle?: (id: string) => Promise<boolean>;
  open: (request: OpenTerminalSessionRequest) => Promise<TerminalSession | null>;
  reconnect: (id: string) => Promise<boolean>;
  stopReconnect: (id: string) => Promise<boolean>;
  // answerHostKey answers whether to trust the key a connecting session asks about (its hostKeyQuestion).
  answerHostKey: (id: string, fingerprint: string, accept: boolean) => Promise<boolean>;
  close: (id: string) => Promise<void>;
  closeAll: () => Promise<void>;
  refresh: () => Promise<void>;
  // markExited records how a session ended as soon as its stream says so,
  // before the next list shows it.
  markExited: (id: string, exit: ExitReport) => void;
  moveToWorkspace: (id: string, workspaceId: string) => Promise<boolean>;
};

// noSessions is what is handed out before the engine in use answers. It is one
// array, so that hooks depending on the list do not run again on every render.
const noSessions: TerminalSession[] = [];

export function useTerminalSessions(
  api: TerminalSessionsApi,
  translate: Translate,
  enabled = true,
): TerminalSessionsState {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [maxSessions, setMaxSessions] = useState(0);
  const [restoreOffer, setRestoreOffer] = useState<string | undefined>(undefined);
  const [pendingOperations, setPendingOperations] = useState(0);
  const [pendingOpens, setPendingOpens] = useState(0);
  const [problem, setProblem] = useState("");
  const [listFailure, setListFailure] = useState<{ api: TerminalSessionsApi; error: ApiError; at: Date } | null>(null);
  // The terminal area must never attribute the previous engine's refusal
  // to the newly selected one, even before the reset effect has run.
  const currentFailure = listFailure?.api === api ? listFailure : null;
  const remoteRefusal = currentFailure?.error ?? null;
  const remoteRefusalAt = currentFailure?.at ?? null;
  // answeredApi is the engine whose answer `sessions` holds. The previous
  // engine's list is not handed out under a new one, not even in the render
  // that switches, before the reset below has run.
  const [answeredApi, setAnsweredApi] = useState<TerminalSessionsApi | null>(null);
  const loaded = answeredApi === api;
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
    listed: TerminalSessionList,
    isCurrent: IsCurrentRequest,
  ): boolean => {
    if (!isCurrent()) return false;
    // A mutation response describes state after the requested change. Retire
    // every list request that started before this response so a slow poll
    // cannot resurrect a closed session or restore an old title/state.
    refreshGeneration.retire();
    setSessions(listed.sessions);
    setMaxSessions(listed.maxSessions);
    setRestoreOffer(listed.restoreOffer);
    setAnsweredApi(api);
    return true;
  }, [api, refreshGeneration]);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    const isCurrent = refreshGeneration.begin();
    try {
      const listed = await api.terminalSessions();
      if (!isCurrent()) return;
      setSessions(listed.sessions);
      setMaxSessions(listed.maxSessions);
      setRestoreOffer(listed.restoreOffer);
      setListFailure(null);
    } catch (error) {
      // A run-target engine that cannot be reached would otherwise look like
      // one with no sessions. This machine's own list keeps failing quietly:
      // the next poll reads it again.
      if (isCurrent() && error instanceof ApiError && error.code.startsWith("remote_engine_")) {
        setListFailure({ api, error, at: new Date() });
      }
    } finally {
      if (isCurrent()) setAnsweredApi(api);
    }
  }, [api, enabled, refreshGeneration]);

  // applyMutation runs one request whose response is the listing after the
  // change. A response that lost the race to a newer mutation is discarded in
  // favour of a fresh list. Callers only say what to show when it fails.
  const applyMutation = useCallback(async (
    request: () => Promise<TerminalSessionList>,
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

  // A different api is another engine: what the last one listed is not shown
  // while the new list is read.
  useEffect(() => {
    refreshGeneration.retire();
    mutationGeneration.retire();
    setSessions([]);
    setMaxSessions(0);
    setRestoreOffer(undefined);
    setProblem("");
    setListFailure(null);
    // An open still waiting belongs to the previous engine; the new one is not
    // being connected to.
    setPendingOpens(0);
  }, [api, mutationGeneration, refreshGeneration]);

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
  // background; that is precisely when delivering them is useful. An empty or
  // refused list is read again too: another browser, a machine that uses this
  // engine as its run target, or `sshc terminal create` can open a session
  // here, and an engine that restarted for an update comes back with none.
  // That is only while the page is shown: each read may wait on an SSH
  // connection to a host that does not answer, and nothing in a hidden page
  // needs it.
  usePolling(refresh, {
    intervalMs: connectionInProgress ? connectingPollIntervalMs : sessionListPollIntervalMs,
    enabled,
    whileHidden: sessions.length > 0,
  });

  const open = useCallback(
    async (request: OpenTerminalSessionRequest): Promise<TerminalSession | null> => {
      beginOperation();
      setPendingOpens((current) => current + 1);
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
        // A run target that could not be reached is shown where the session
        // would have appeared, as when its list cannot be read, rather than
        // only as a line in the sidebar.
        if (error instanceof ApiError && error.code.startsWith("remote_engine_")) {
          setListFailure({ api, error, at: new Date() });
        }
        return null;
      } finally {
        setPendingOpens((current) => Math.max(0, current - 1));
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

  const answerHostKey = useCallback(
    async (id: string, fingerprint: string, accept: boolean): Promise<boolean> => {
      setProblem("");
      return applyMutation(() => api.answerTerminalHostKey(id, { fingerprint, accept }), async (error) => {
        // The question closed first (answered in the terminal, or the connection went on); the list shows where it is.
        if (failureCode(error) === "terminal_no_host_key_question") {
          await refresh();
          return;
        }
        await reportConnectionFailure(error);
      });
    },
    [api, applyMutation, refresh, reportConnectionFailure],
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

  const moveToWorkspace = useCallback(
    (id: string, workspaceId: string): Promise<boolean> =>
      applyMutation(() => api.moveTerminalSession(id, workspaceId), () => setProblem(translate("workspace.moveFailed"))),
    [api, applyMutation, translate],
  );

  const markExited = useCallback((id: string, exit: ExitReport) => {
    beginMutation();
    setSessions((current) =>
      current.map((session) =>
        session.id === id && session.exited === undefined
          ? { ...session, state: "exited", exited: { ...exit, at: "" } }
          : session,
      ),
    );
  }, [beginMutation]);

  return {
    sessions: loaded ? sessions : noSessions,
    maxSessions, busy, problem, remoteRefusal, remoteRefusalAt, opening: pendingOpens > 0, loaded, restoreOffer,
    rename, unpinTitle, open, reconnect, stopReconnect, answerHostKey, close, closeAll, refresh, markExited, moveToWorkspace,
  };
}

export function terminalProblemKey(code: string): MessageKey {
  switch (code) {
    case "terminal_session_limit":
      return "terminal.limitRefused";
    case "alias_unresolvable":
      return "terminal.unresolvable";
    case "remote_working_directory_unsupported":
      return "terminal.remoteWorkingDirectoryUnsupported";
    case "local_working_directory_unavailable":
      return "terminal.localWorkingDirectoryUnavailable";
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
    case "remote_engine_vault_locked":
      return "terminal.remoteVaultLocked";
    case "remote_engine_refused":
    case "remote_engine_unreachable":
    case "remote_engine_not_running":
    case "remote_engine_unproven":
    case "remote_engine_connect_failed":
      return "terminal.remoteEngineUnavailable";
    case "remote_engine_incompatible":
      return "terminal.remoteIncompatible";
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

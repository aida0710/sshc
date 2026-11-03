import { useEffect, useMemo, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { sftpProblemText } from "./sftpProblemText";
import type { RemoteEntry, SFTPConnectionAttempt } from "./api";
import { watchConnectionAttempt, type ConnectionAttemptWatch } from "./sftpConnectionAttempt";
import { localHostAlias } from "./localHost";
import { sourceFor, type SFTPListing } from "./sftpSource";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import type { SFTPLocation } from "./sftpLocation";

export type { SFTPLocation } from "./sftpLocation";
// A restored location, plus whether it was live when the tab was put away. A
// tab moved between panes was connected a moment ago and reopens at once.
export type RestoredSFTPLocation = SFTPLocation & { connect?: boolean };

type LoadOptions = {
  // Which host to list. Defaults to the current one; a target link names it
  // before the state has caught up.
  alias?: string;
  // Re-read the directory in place: no history entry, and the pane keeps
  // whatever it had open over the rows.
  refresh?: boolean;
  // Whether the directory joins the back/forward history. Off for refreshes
  // and for the history moves themselves.
  record?: boolean;
};

// What every pane does with a source: which host and directory it shows, the
// rows it read, whether it is reading, back/forward history, and the one
// generation counter that lets a slow answer for a directory the user has
// already left be dropped. Everything about selection, menus, editing or
// transfers stays outside.
export function useSFTPBrowser({
  aliases,
  initialLocation = null,
  onLocationChange = () => undefined,
  onLoaded,
  onReset,
}: {
  aliases: string[];
  // Where a restored tab should reopen. Applied once, when the declared
  // aliases have arrived and can vouch for the host. A source that needs no
  // connection is read right away; one that does waits for connect().
  initialLocation?: RestoredSFTPLocation | null;
  onLocationChange?: (alias: string, path: string) => void;
  // After every successful listing. `changed` says whether it is a different
  // directory (or host) from the one shown before.
  onLoaded?: (listing: SFTPListing, info: { refresh: boolean; changed: boolean }) => void;
  // When the host changes and every per-host state in the pane must go.
  onReset?: () => void;
}) {
  const t = useTranslate();
  const [alias, setAlias] = useState("");
  const [path, setPath] = useState("");
  const [home, setHome] = useState<string | undefined>(undefined);
  const [connected, setConnected] = useState(false);
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  // connection is what the engine reported while this pane connected: the stage
  // while it waits, and the log it keeps showing when the connection failed.
  const [connection, setConnection] = useState<SFTPConnectionAttempt | null>(null);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const [navigation, setNavigation] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const loadGeneration = useRequestGeneration();
  // connecting is the listing that opens the SSH connection, while it waits. It may wait on a host key question,
  // which only this pane can answer.
  const connecting = useRef<{ watch: ConnectionAttemptWatch; abort: AbortController } | null>(null);
  const requestedPath = useRef("");
  const openedInitialLocation = useRef(false);
  const reportLocation = useRef(onLocationChange);
  reportLocation.current = onLocationChange;
  const latest = useRef({ alias, path, connected });
  latest.current = { alias, path, connected };
  const source = useMemo(() => sourceFor(alias), [alias]);

  function knownHost(candidate: string): boolean {
    return candidate === localHostAlias || aliases.includes(candidate);
  }

  // Once the pane moves on (another listing, another host, the pane is taken down), nobody answers the connecting
  // listing's host key question. It is ended rather than left waiting in the engine, which would keep the request,
  // the half-open SSH connection and the polling open for good.
  function abandonConnecting() {
    connecting.current?.abort.abort();
    connecting.current = null;
  }

  function beginRequest() {
    abandonConnecting();
    return loadGeneration.begin();
  }

  function retireRequests() {
    abandonConnecting();
    loadGeneration.retire();
  }

  // Taking the pane down (closing its tab, opening another section) ends a listing still connecting. StrictMode in
  // development takes the pane down and puts it back at once; the restored location then opens again, as it does for
  // a pane put back.
  useEffect(() => () => {
    abandonConnecting();
    openedInitialLocation.current = false;
  }, []);

  async function load(nextPath: string = latest.current.path, options: LoadOptions = {}): Promise<RemoteEntry[] | null> {
    const targetAlias = options.alias ?? latest.current.alias;
    const target = sourceFor(targetAlias);
    const isCurrent = beginRequest();
    if (target === null) {
      setBusy(false);
      return null;
    }
    const refresh = options.refresh === true;
    const record = options.record ?? !refresh;
    requestedPath.current = nextPath;
    setBusy(true);
    setPendingPath(nextPath);
    setProblem("");
    // A listing for a host this pane is not connected to opens the SSH
    // connection, which the engine reports while it lasts.
    const opensConnection = !target.local && (!latest.current.connected || targetAlias !== latest.current.alias);
    const connection = opensConnection ? {
      watch: watchConnectionAttempt(targetAlias, (reading) => { if (isCurrent()) setConnection(reading); }),
      abort: new AbortController(),
    } : null;
    if (connection !== null) {
      connecting.current = connection;
      setConnection(null);
    }
    try {
      const listing = await (connection === null ? target.list(nextPath)
        : target.list(nextPath, { attempt: connection.watch.attempt, signal: connection.abort.signal }));
      void connection?.watch.stop({ readFinal: false });
      if (!isCurrent()) return null;
      connecting.current = null;
      setConnection(null);
      const changed = listing.path !== latest.current.path || targetAlias !== latest.current.alias;
      setPath(listing.path);
      setHome(listing.home);
      setConnected(true);
      setEntries(listing.entries);
      reportLocation.current(targetAlias, listing.path);
      if (record) {
        setNavigation((current) => {
          if (current.paths[current.index] === listing.path) return current;
          const paths = [...current.paths.slice(0, current.index + 1), listing.path];
          return { paths, index: paths.length - 1 };
        });
      }
      onLoaded?.(listing, { refresh, changed });
      return listing.entries;
    } catch (error) {
      // An ended listing belongs to a pane that moved on; there is nothing left to show of it.
      await connection?.watch.stop({ readFinal: !connection.abort.signal.aborted });
      if (!isCurrent()) return null;
      connecting.current = null;
      const code = failureCode(error);
      if (code === "invalid_request") {
        // A listing's only free input is the path typed into the pane, so the
        // engine refusing the request means it cannot open that path.
        setProblem(t("sftp.pathCannotOpen"));
      } else if (!target.local && code === "sftp_failed") {
        setProblem(t("sftp.connectionFailed"));
      } else {
        setProblem(sftpProblemText(t, error, target.local ? "sftp.problem.failed" : "sftp.connectionFailed"));
      }
      return null;
    } finally {
      if (isCurrent()) {
        setBusy(false);
        setPendingPath(null);
      }
    }
  }

  // Runs some other request against the source (a search, say) under the same
  // busy state, loading overlay and staleness rule as a listing. Resolves to
  // null when it failed or when the user moved on before it answered.
  async function track<T>(pending: string, work: () => Promise<T>): Promise<T | null> {
    const isCurrent = beginRequest();
    setBusy(true);
    setPendingPath(pending);
    setProblem("");
    try {
      const result = await work();
      return isCurrent() ? result : null;
    } catch (error) {
      if (isCurrent()) {
        setProblem(sftpProblemText(t, error));
      }
      return null;
    } finally {
      if (isCurrent()) {
        setBusy(false);
        setPendingPath(null);
      }
    }
  }

  // Empties the pane for another host without reading it; the caller reads
  // the directory it wants.
  function resetForHost(nextAlias: string) {
    // Invalidate every request started for the previous host before React runs
    // the alias effect. Keeping its rows visible would also let an action for
    // host A be submitted with host B's alias during the hand-off render.
    retireRequests();
    reportLocation.current(nextAlias, "");
    setAlias(nextAlias);
    setConnected(false);
    setPath("");
    setHome(undefined);
    setEntries([]);
    setNavigation({ paths: [], index: -1 });
    setProblem("");
    setConnection(null);
    setBusy(false);
    setPendingPath(null);
    onReset?.();
    latest.current = { alias: nextAlias, path: "", connected: false };
  }

  // Choosing a host is the request to use it, so an SSH host connects here
  // without a further Connect. Only a restored tab waits for Connect: nobody
  // chose its host on this visit.
  function selectHost(nextAlias: string) {
    resetForHost(nextAlias);
    void load("", { alias: nextAlias });
  }

  useEffect(() => {
    if (initialLocation === null || openedInitialLocation.current) return;
    const restored = sourceFor(initialLocation.alias);
    if (restored === null || !knownHost(initialLocation.alias) ||
        (initialLocation.path !== "" && !restored.acceptsPath(initialLocation.path))) return;
    openedInitialLocation.current = true;
    retireRequests();
    setAlias(initialLocation.alias);
    setPath(initialLocation.path);
    setConnected(false);
    latest.current = { alias: initialLocation.alias, path: initialLocation.path, connected: false };
    // Tabs restored from storage remember where they were, but never open an
    // SSH connection until the user explicitly presses Connect. The engine's
    // own disk needs no connection, and a tab that was connected when it moved
    // between panes reopens at once.
    if (!restored.can.connect || initialLocation.connect === true) void load(initialLocation.path, { alias: initialLocation.alias });
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [aliases, initialLocation?.alias, initialLocation?.path]);

  // answerHostKey answers the question the connecting listing asks about an unknown host key (connection's
  // hostKeyQuestion). False when no listing of this pane waits on one, or the answer did not arrive: the
  // application-wide failure notice says why, and the question stays for another press.
  async function answerHostKey(fingerprint: string, accept: boolean): Promise<boolean> {
    const watch = connecting.current?.watch;
    if (watch === undefined) return false;
    return watch.answerHostKey(fingerprint, accept).catch(() => false);
  }

  async function navigateHistory(offset: -1 | 1): Promise<void> {
    const nextIndex = navigation.index + offset;
    const destination = navigation.paths[nextIndex];
    if (destination === undefined) return;
    const loaded = await load(destination, { record: false });
    if (loaded === null) return;
    setNavigation((current) => current.paths[nextIndex] === destination ? { ...current, index: nextIndex } : current);
  }

  const canBack = navigation.index > 0;
  const canForward = navigation.index >= 0 && navigation.index < navigation.paths.length - 1;
  const atRoot = source === null || path === "" || source.isRoot(path);
  // A listing that failed says so where the rows would be, with the retry next
  // to it. Repeating the same sentence in the banner above would be two voices
  // for one fact.
  const listingFailed = problem !== "" && alias !== "" && entries.length === 0;

  return {
    alias,
    source,
    path,
    home,
    connected,
    entries,
    busy,
    problem,
    setProblem,
    connection,
    pendingPath,
    listingFailed,
    atRoot,
    canBack,
    canForward,
    load,
    track,
    // Callers that write to the source observe this before the write and
    // check it after, so that a host switched mid-flight cannot receive the
    // result.
    generation: loadGeneration,
    selectHost,
    resetForHost,
    answerHostKey,
    connect: () => load(latest.current.path),
    retry: () => load(requestedPath.current),
    refresh: () => load(latest.current.path, { refresh: true }),
    back: () => navigateHistory(-1),
    forward: () => navigateHistory(1),
    goHome: () => load(""),
    goRoot: () => load(source === null ? "/" : source.rootOf(latest.current.path)),
    openParent: () => load(source === null ? "/" : source.parentOf(latest.current.path)),
    crumbs: source === null || path === "" ? [] : source.crumbs(path, home),
  };
}

export type SFTPBrowserModel = ReturnType<typeof useSFTPBrowser>;

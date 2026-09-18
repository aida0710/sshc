import { useEffect, useMemo, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { RemoteEntry } from "./api";
import { localHostAlias } from "./localHost";
import { sourceFor, type SFTPListing } from "./sftpSource";

export type SFTPLocation = { alias: string; path: string };
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
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const [navigation, setNavigation] = useState<{ paths: string[]; index: number }>({ paths: [], index: -1 });
  const loadGeneration = useRef(0);
  const requestedPath = useRef("");
  const openedInitialLocation = useRef(false);
  const reportLocation = useRef(onLocationChange);
  reportLocation.current = onLocationChange;
  const latest = useRef({ alias, path });
  latest.current = { alias, path };
  const source = useMemo(() => sourceFor(alias), [alias]);

  function knownHost(candidate: string): boolean {
    return candidate === localHostAlias || aliases.includes(candidate);
  }

  async function load(nextPath: string = latest.current.path, options: LoadOptions = {}): Promise<RemoteEntry[] | null> {
    const targetAlias = options.alias ?? latest.current.alias;
    const target = sourceFor(targetAlias);
    const generation = ++loadGeneration.current;
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
    try {
      const listing = await target.list(nextPath);
      if (generation !== loadGeneration.current) return null;
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
      if (generation !== loadGeneration.current) return null;
      const code = failureCode(error);
      const fallback = error instanceof Error ? error.message : "sftp_failed";
      setProblem(target.local
        ? code || fallback
        : code === "sftp_failed" ? t("sftp.connectionFailed") : code || (error instanceof Error ? error.message : t("sftp.connectionFailed")));
      return null;
    } finally {
      if (generation === loadGeneration.current) {
        setBusy(false);
        setPendingPath(null);
      }
    }
  }

  // Runs some other request against the source (a search, say) under the same
  // busy state, loading overlay and staleness rule as a listing. Resolves to
  // null when it failed or when the user moved on before it answered.
  async function track<T>(pending: string, work: () => Promise<T>): Promise<T | null> {
    const generation = ++loadGeneration.current;
    setBusy(true);
    setPendingPath(pending);
    setProblem("");
    try {
      const result = await work();
      return generation === loadGeneration.current ? result : null;
    } catch (error) {
      if (generation === loadGeneration.current) {
        setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
      }
      return null;
    } finally {
      if (generation === loadGeneration.current) {
        setBusy(false);
        setPendingPath(null);
      }
    }
  }

  function selectHost(nextAlias: string) {
    // Invalidate every request started for the previous host before React runs
    // the alias effect. Keeping its rows visible would also let an action for
    // host A be submitted with host B's alias during the hand-off render.
    loadGeneration.current += 1;
    reportLocation.current(nextAlias, "");
    setAlias(nextAlias);
    setConnected(false);
    setPath("");
    setHome(undefined);
    setEntries([]);
    setNavigation({ paths: [], index: -1 });
    setProblem("");
    setBusy(false);
    setPendingPath(null);
    onReset?.();
    latest.current = { alias: nextAlias, path: "" };
    const next = sourceFor(nextAlias);
    if (next !== null && !next.can.connect) void load("", { alias: nextAlias });
  }

  useEffect(() => {
    if (initialLocation === null || openedInitialLocation.current) return;
    const restored = sourceFor(initialLocation.alias);
    if (restored === null || !knownHost(initialLocation.alias) ||
        (initialLocation.path !== "" && !restored.acceptsPath(initialLocation.path))) return;
    openedInitialLocation.current = true;
    loadGeneration.current += 1;
    setAlias(initialLocation.alias);
    setPath(initialLocation.path);
    setConnected(false);
    latest.current = { alias: initialLocation.alias, path: initialLocation.path };
    // Tabs restored from storage remember where they were, but never open an
    // SSH connection until the user explicitly presses Connect. The engine's
    // own disk needs no connection, and a tab that was connected when it moved
    // between panes reopens at once.
    if (!restored.can.connect || initialLocation.connect === true) void load(initialLocation.path, { alias: initialLocation.alias });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aliases, initialLocation?.alias, initialLocation?.path]);

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
    pendingPath,
    listingFailed,
    atRoot,
    canBack,
    canForward,
    load,
    track,
    // Callers that write to the source read this before and after, so that a
    // host switched mid-flight cannot receive the result.
    generation: loadGeneration,
    selectHost,
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

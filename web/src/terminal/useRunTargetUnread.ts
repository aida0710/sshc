import { useCallback, useEffect, useRef, useState } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { runTargetTerminalSessionsApi } from "../api/terminalSessions";
import { usePolling } from "../ui/usePolling";
import { noteNotificationVersions, type NotificationVersions } from "./runTargetUnread";

// runTargetUnreadPollMs is how often the run targets not on screen are read for new notifications. Nobody
// is looking at them, so a mark that comes a few seconds late costs nothing, and each read crosses SSH.
const runTargetUnreadPollMs = 15_000;

// useRunTargetUnread marks the run targets other than the selected one whose terminals sent a notification
// (OSC 9, 99 or 777) since this browser last looked. It reads only run targets this machine's engine is
// connected to, so polling never starts a connection or asks for a password. Choosing a run target clears
// its mark: its sessions then show their own unread marks.
export function useRunTargetUnread({ api, selected, enabled }: {
  api: RemoteEnginesApi;
  selected: string | null;
  enabled: boolean;
}): ReadonlySet<string> {
  const [unread, setUnread] = useState<ReadonlySet<string>>(() => new Set());
  const seen = useRef(new Map<string, NotificationVersions>());
  const shown = useRef(selected);

  useEffect(() => {
    // The run target now on screen starts over from what it shows; the one left starts over from its next read.
    if (shown.current !== null) seen.current.delete(shown.current);
    if (selected !== null) seen.current.delete(selected);
    shown.current = selected;
    if (selected === null) return;
    setUnread((current) => {
      if (!current.has(selected)) return current;
      const next = new Set(current);
      next.delete(selected);
      return next;
    });
  }, [selected]);

  const read = useCallback(async (isCurrent: () => boolean) => {
    const listed = await api.list();
    const watched = listed.engines.filter((engine) => engine.connected && engine.alias !== selected).map((engine) => engine.alias);
    for (const alias of [...seen.current.keys()]) {
      if (!watched.includes(alias)) seen.current.delete(alias);
    }
    const notified: string[] = [];
    await Promise.all(watched.map(async (alias) => {
      const sessions = await runTargetTerminalSessionsApi(alias).terminalSessions().catch(() => null);
      if (sessions === null || !isCurrent()) return;
      const noted = noteNotificationVersions(seen.current.get(alias), sessions.sessions);
      seen.current.set(alias, noted.seen);
      if (noted.notified) notified.push(alias);
    }));
    if (!isCurrent() || notified.length === 0) return;
    setUnread((current) => new Set([...current, ...notified]));
  }, [api, selected]);

  usePolling((isCurrent) => read(isCurrent), { intervalMs: runTargetUnreadPollMs, enabled, immediately: true });
  return unread;
}

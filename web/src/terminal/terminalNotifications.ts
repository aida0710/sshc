import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";
import { terminalDisplayTitle } from "./terminalPresentation";

// A program inside the terminal asked for a desktop notification through the
// standard OSC 9, OSC 99 or OSC 777 sequences. The engine records it on the
// session; the browser decides how to surface it.
export type TerminalNotification = {
  title: string;
  body: string;
};

export type UnreadSessions = ReadonlySet<string>;

export const notificationSoundPresets = ["none", "gentle", "bell", "pulse"] as const;
export type NotificationSoundPreset = typeof notificationSoundPresets[number];
export type NotificationSoundPreferences = {
  sound: NotificationSoundPreset;
  volume: number;
};

const soundPreferenceKey = "sshc.terminal-notification-sound.v1";
const audioContexts = new WeakMap<Window, AudioContext>();
export const defaultNotificationSoundPreferences: NotificationSoundPreferences = {
  sound: "bell",
  volume: 60,
};

export type BrowserNotificationPermission = NotificationPermission | "unsupported";

type NotificationWindow = Window & { Notification?: typeof Notification };

function notificationAPI(target: Window): typeof Notification | null {
  const candidate = (target as NotificationWindow).Notification;
  return typeof candidate === "function" && typeof candidate.requestPermission === "function"
    ? candidate
    : null;
}

export function browserNotificationPermission(target: Window = window): BrowserNotificationPermission {
  return notificationAPI(target)?.permission ?? "unsupported";
}

export async function requestBrowserNotificationPermission(
  target: Window = window,
): Promise<BrowserNotificationPermission> {
  const api = notificationAPI(target);
  if (api === null) return "unsupported";
  if (api.permission !== "default") return api.permission;
  return api.requestPermission();
}

export function showBrowserNotification(
  notification: TerminalNotification & { tag: string; onClick?: () => void },
  target: Window = window,
): boolean {
  const api = notificationAPI(target);
  if (api === null || api.permission !== "granted") return false;
  try {
    const shown = new api(notification.title, { body: notification.body, tag: notification.tag });
    if (notification.onClick !== undefined) {
      shown.onclick = () => {
        try {
          target.focus();
        } catch {
          // Some wrappers do not expose a focusable browser window.
        }
        notification.onClick?.();
        shown.close();
      };
    }
    return true;
  } catch {
    // Native wrappers and browsers can revoke delivery between checks.
    return false;
  }
}

function isSoundPreset(value: unknown): value is NotificationSoundPreset {
  return typeof value === "string" && (notificationSoundPresets as readonly string[]).includes(value);
}

export function loadNotificationSoundPreferences(
  target: Pick<Window, "localStorage"> = window,
): NotificationSoundPreferences {
  try {
    const stored = JSON.parse(target.localStorage.getItem(soundPreferenceKey) ?? "null") as unknown;
    if (stored === null || typeof stored !== "object") return defaultNotificationSoundPreferences;
    const candidate = stored as Partial<NotificationSoundPreferences>;
    const volume = typeof candidate.volume === "number" && Number.isFinite(candidate.volume)
      ? Math.max(0, Math.min(100, Math.round(candidate.volume)))
      : defaultNotificationSoundPreferences.volume;
    return {
      sound: isSoundPreset(candidate.sound) ? candidate.sound : defaultNotificationSoundPreferences.sound,
      volume,
    };
  } catch {
    return defaultNotificationSoundPreferences;
  }
}

export function saveNotificationSoundPreferences(
  preferences: NotificationSoundPreferences,
  target: Pick<Window, "localStorage"> = window,
) {
  try {
    target.localStorage.setItem(soundPreferenceKey, JSON.stringify(preferences));
  } catch {
    // Private browsing policies may disable localStorage.
  }
}

function notificationAudioContext(target: Window): AudioContext | null {
  const existing = audioContexts.get(target);
  if (existing !== undefined && existing.state !== "closed") return existing;
  const Audio = (target as Window & { AudioContext?: typeof AudioContext }).AudioContext;
  if (Audio === undefined) return null;
  try {
    const created = new Audio();
    audioContexts.set(target, created);
    return created;
  } catch {
    return null;
  }
}

export function primeNotificationSound(target: Window = window): boolean {
  const audio = notificationAudioContext(target);
  if (audio === null) return false;
  if (audio.state === "suspended") void audio.resume().catch(() => undefined);
  return true;
}

// nextTerminalNotification returns the notification to surface for a session
// whose notificationVersion moved past what this browser had already seen.
// The first observation of a session is history, never a new event.
export function nextTerminalNotification(
  t: Translate,
  session: TerminalSession,
  previousVersion: number | undefined,
): TerminalNotification | null {
  if (previousVersion === undefined || (session.notificationVersion ?? 0) <= previousVersion) return null;
  const last = session.lastNotification;
  if (last === undefined) return null;
  const displayTitle = terminalDisplayTitle(session);
  const alias = session.kind === "ssh" ? session.alias : undefined;
  const subject = alias === undefined || alias === displayTitle ? displayTitle : `${displayTitle}（${alias}）`;
  // The browser notification names the pane so several agents stay apart;
  // the program's own title becomes the first line of the body.
  const body = last.title !== "" && last.body !== ""
    ? `${last.title}\n${last.body}`
    : last.title || last.body || t("terminal.notificationFallback");
  return { title: subject, body };
}

export function playNotificationSound(
  preferences: NotificationSoundPreferences = loadNotificationSoundPreferences(),
  target: Window = window,
): boolean {
  const preset = preferences.sound;
  if (preset === "none" || preferences.volume <= 0) return false;
  const audio = notificationAudioContext(target);
  if (audio === null) return false;
  const schedule = () => {
    const gain = audio.createGain();
    const plan = preset === "bell"
      ? { type: "triangle" as OscillatorType, frequencies: [880, 1174], duration: 0.32, peak: 0.12 }
      : preset === "pulse"
        ? { type: "sine" as OscillatorType, frequencies: [660, 660], duration: 0.24, peak: 0.1 }
        : { type: "sine" as OscillatorType, frequencies: [520, 660], duration: 0.22, peak: 0.07 };
    const peak = Math.max(0.0001, plan.peak * preferences.volume / 100);
    gain.gain.setValueAtTime(0.0001, audio.currentTime);
    gain.gain.exponentialRampToValueAtTime(peak, audio.currentTime + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + plan.duration);
    gain.connect(audio.destination);
    plan.frequencies.forEach((frequency, index) => {
      const oscillator = audio.createOscillator();
      oscillator.type = plan.type;
      oscillator.frequency.value = frequency;
      oscillator.connect(gain);
      oscillator.start(audio.currentTime + index * plan.duration / 2);
      oscillator.stop(audio.currentTime + plan.duration);
    });
  };
  try {
    if (audio.state === "suspended") void audio.resume().then(schedule).catch(() => undefined);
    else schedule();
    return true;
  } catch {
    return false;
  }
}

// useTerminalNotifications turns notificationVersion changes into unread marks,
// sounds and browser notifications. A pane the user is looking at never
// becomes unread; a hidden or background pane does, until it is focused.
export function useTerminalNotifications(
  sessions: TerminalSession[],
  activeSessionId: string | null,
  t: Translate,
  onOpenSession: (id: string) => void,
): UnreadSessions {
  const seen = useRef(new Map<string, number>());
  const activeSession = useRef(activeSessionId);
  const openSession = useRef(onOpenSession);
  const [unread, setUnread] = useState<Set<string>>(() => new Set());
  activeSession.current = activeSessionId;
  openSession.current = onOpenSession;

  useEffect(() => {
    const prime = () => {
      primeNotificationSound();
      window.removeEventListener("pointerdown", prime, true);
      window.removeEventListener("keydown", prime, true);
    };
    window.addEventListener("pointerdown", prime, true);
    window.addEventListener("keydown", prime, true);
    return () => {
      window.removeEventListener("pointerdown", prime, true);
      window.removeEventListener("keydown", prime, true);
    };
  }, []);

  const markActiveRead = useCallback(() => {
    const id = activeSession.current;
    if (id === null || document.hidden) return;
    setUnread((current) => {
      if (!current.has(id)) return current;
      const next = new Set(current);
      next.delete(id);
      return next;
    });
  }, []);

  useEffect(() => {
    markActiveRead();
    window.addEventListener("focus", markActiveRead);
    document.addEventListener("visibilitychange", markActiveRead);
    return () => {
      window.removeEventListener("focus", markActiveRead);
      document.removeEventListener("visibilitychange", markActiveRead);
    };
  }, [activeSessionId, markActiveRead]);

  useEffect(() => {
    const currentIDs = new Set(sessions.map((session) => session.id));
    for (const id of seen.current.keys()) {
      if (!currentIDs.has(id)) seen.current.delete(id);
    }
    for (const session of sessions) {
      const previous = seen.current.get(session.id);
      seen.current.set(session.id, session.notificationVersion ?? 0);
      if (previous === undefined) continue;
      const notification = nextTerminalNotification(t, session, previous);
      if (notification === null) continue;
      if (document.hidden || session.id !== activeSession.current) {
        setUnread((current) => {
          if (current.has(session.id)) return current;
          const next = new Set(current);
          next.add(session.id);
          return next;
        });
      }
      // Sounds and browser notifications only reach a user who is away;
      // a visible app already shows the unread mark.
      if (!document.hidden) continue;
      playNotificationSound();
      showBrowserNotification({
        title: notification.title,
        body: notification.body,
        tag: `sshc-terminal-${session.id}`,
        onClick: () => openSession.current(session.id),
      });
    }
    setUnread((current) => {
      if ([...current].every((id) => currentIDs.has(id))) return current;
      return new Set([...current].filter((id) => currentIDs.has(id)));
    });
  }, [sessions, t]);

  return unread;
}

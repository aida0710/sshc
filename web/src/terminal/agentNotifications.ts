import { useCallback, useEffect, useRef, useState } from "react";
import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";
import { terminalDisplayTitle } from "./agentPresentation";

export type AgentNotification = {
  kind: "attention" | "completed";
  title: string;
  body: string;
};

export type AgentUnread = AgentNotification["kind"];
export type AgentUnreadBySession = ReadonlyMap<string, AgentUnread>;

export const agentSoundPresets = ["none", "gentle", "bell", "pulse"] as const;
export type AgentSoundPreset = typeof agentSoundPresets[number];
export type AgentSoundPreferences = {
  attention: AgentSoundPreset;
  completed: AgentSoundPreset;
  volume: number;
};

const soundPreferenceKey = "sshc.agent-notification-sounds.v1";
const audioContexts = new WeakMap<Window, AudioContext>();
export const defaultAgentSoundPreferences: AgentSoundPreferences = {
  attention: "bell",
  completed: "gentle",
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
  notification: Pick<AgentNotification, "title" | "body"> & { tag: string; onClick?: () => void },
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

function isAgentSoundPreset(value: unknown): value is AgentSoundPreset {
  return typeof value === "string" && (agentSoundPresets as readonly string[]).includes(value);
}

export function loadAgentSoundPreferences(target: Pick<Window, "localStorage"> = window): AgentSoundPreferences {
  try {
    const stored = JSON.parse(target.localStorage.getItem(soundPreferenceKey) ?? "null") as unknown;
    if (stored === null || typeof stored !== "object") return defaultAgentSoundPreferences;
    const candidate = stored as Partial<AgentSoundPreferences>;
    const volume = typeof candidate.volume === "number" && Number.isFinite(candidate.volume)
      ? Math.max(0, Math.min(100, Math.round(candidate.volume)))
      : defaultAgentSoundPreferences.volume;
    return {
      attention: isAgentSoundPreset(candidate.attention)
        ? candidate.attention
        : defaultAgentSoundPreferences.attention,
      completed: isAgentSoundPreset(candidate.completed)
        ? candidate.completed
        : defaultAgentSoundPreferences.completed,
      volume,
    };
  } catch {
    return defaultAgentSoundPreferences;
  }
}

export function saveAgentSoundPreferences(
  preferences: AgentSoundPreferences,
  target: Pick<Window, "localStorage"> = window,
) {
  try {
    target.localStorage.setItem(soundPreferenceKey, JSON.stringify(preferences));
  } catch {
    // Private browsing policies may disable localStorage.
  }
}

function agentAudioContext(target: Window): AudioContext | null {
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

export function primeAgentSound(target: Window = window): boolean {
  const audio = agentAudioContext(target);
  if (audio === null) return false;
  if (audio.state === "suspended") void audio.resume().catch(() => undefined);
  return true;
}

export function nextAgentNotification(
  t: Translate,
  session: TerminalSession,
  previousSignalVersion: number | undefined,
): AgentNotification | null {
  const signal = session.agent?.lastSignal;
  if (signal === undefined || previousSignalVersion === undefined || session.agent === undefined) return null;
  if (session.agent.signalVersion <= previousSignalVersion) return null;
  const displayTitle = terminalDisplayTitle(session);
  const alias = session.kind === "ssh" ? session.alias : undefined;
  const subject = alias === undefined || alias === displayTitle ? displayTitle : `${displayTitle}（${alias}）`;
  return {
    kind: signal.kind,
    title: "sshc",
    body: signal.kind === "attention"
      ? t("terminal.agentNotificationAttention", { subject })
      : t("terminal.agentNotificationCompleted", { subject }),
  };
}

export function playAgentSound(
  kind: AgentNotification["kind"],
  preferences: AgentSoundPreferences = loadAgentSoundPreferences(),
  target: Window = window,
): boolean {
  const preset = preferences[kind];
  if (preset === "none" || preferences.volume <= 0) return false;
  const audio = agentAudioContext(target);
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

export function useAgentNotifications(
  sessions: TerminalSession[],
  activeSessionId: string | null,
  t: Translate,
  onOpenSession: (id: string) => void,
): AgentUnreadBySession {
  const seen = useRef(new Map<string, number>());
  const activeSession = useRef(activeSessionId);
  const openSession = useRef(onOpenSession);
  const [unread, setUnread] = useState<Map<string, AgentUnread>>(() => new Map());
  activeSession.current = activeSessionId;
  openSession.current = onOpenSession;

  useEffect(() => {
    const prime = () => {
      primeAgentSound();
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
      const next = new Map(current);
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
      const version = session.agent?.signalVersion ?? 0;
      const previous = seen.current.get(session.id);
      seen.current.set(session.id, version);
      // Initial fetch is history, not a new event. Agent unread state is driven
      // exclusively by hook-provided signalVersion/lastSignal; terminal text and
      // generic shell state are deliberately ignored.
      if (previous === undefined) continue;
      const notification = nextAgentNotification(t, session, previous);
      if (notification === null) continue;
      if (document.hidden || session.id !== activeSession.current) {
        setUnread((current) => {
          const next = new Map(current);
          next.set(session.id, notification.kind);
          return next;
        });
      }
      // Preserve the existing background-only browser/sound notification policy.
      if (!document.hidden) continue;
      playAgentSound(notification.kind);
      showBrowserNotification({
        title: notification.title,
        body: notification.body,
        tag: `sshc-agent-${session.id}`,
        onClick: () => openSession.current(session.id),
      });
    }
    setUnread((current) => {
      if ([...current.keys()].every((id) => currentIDs.has(id))) return current;
      return new Map([...current].filter(([id]) => currentIDs.has(id)));
    });
  }, [sessions, t]);

  return unread;
}

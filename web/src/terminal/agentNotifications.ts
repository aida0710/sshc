import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";
import { terminalDisplayTitle } from "./agentPresentation";

export type AgentNotification = {
  kind: "attention" | "completed";
  title: string;
  body: string;
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
  notification: Pick<AgentNotification, "title" | "body"> & { tag: string },
  target: Window = window,
): boolean {
  const api = notificationAPI(target);
  if (api === null || api.permission !== "granted") return false;
  try {
    new api(notification.title, { body: notification.body, tag: notification.tag });
    return true;
  } catch {
    // Native wrappers and browsers can revoke delivery between checks.
    return false;
  }
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

function chime(kind: AgentNotification["kind"]) {
  const Audio = window.AudioContext;
  if (Audio === undefined) return;
  try {
    const audio = new Audio();
    const oscillator = audio.createOscillator();
    const gain = audio.createGain();
    oscillator.type = "sine";
    oscillator.frequency.value = kind === "attention" ? 740 : 520;
    gain.gain.setValueAtTime(0.0001, audio.currentTime);
    gain.gain.exponentialRampToValueAtTime(kind === "attention" ? 0.12 : 0.07, audio.currentTime + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + (kind === "attention" ? 0.28 : 0.18));
    oscillator.connect(gain).connect(audio.destination);
    oscillator.start();
    oscillator.stop(audio.currentTime + (kind === "attention" ? 0.3 : 0.2));
    oscillator.addEventListener("ended", () => void audio.close());
  } catch {
    // Notification delivery is best-effort and must never affect the terminal.
  }
}

export function useAgentNotifications(sessions: TerminalSession[], t: Translate) {
  const seen = useRef(new Map<string, number>());
  useEffect(() => {
    const currentIDs = new Set(sessions.map((session) => session.id));
    for (const id of seen.current.keys()) {
      if (!currentIDs.has(id)) seen.current.delete(id);
    }
    for (const session of sessions) {
      const version = session.agent?.signalVersion ?? 0;
      const previous = seen.current.get(session.id);
      seen.current.set(session.id, version);
      // Initial fetch, foreground use, and focused panes are intentionally silent.
      if (previous === undefined || !document.hidden) continue;
      const notification = nextAgentNotification(t, session, previous);
      if (notification === null) continue;
      chime(notification.kind);
      showBrowserNotification({
        title: notification.title,
        body: notification.body,
        tag: `sshc-agent-${session.id}`,
      });
    }
  }, [sessions, t]);
}

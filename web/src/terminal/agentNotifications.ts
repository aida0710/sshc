import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/integrations";
import type { Translate } from "../i18n/context";
import { terminalDisplayTitle } from "./agentPresentation";

export type AgentNotification = {
  kind: "attention" | "completed";
  title: string;
  body: string;
};

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
      if ("Notification" in window && Notification.permission === "granted") {
        try {
          new Notification(notification.title, { body: notification.body, tag: `sshc-agent-${session.id}` });
        } catch {
          // Native wrappers and browsers can revoke delivery between polls.
        }
      }
    }
  }, [sessions, t]);
}

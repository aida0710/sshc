import { useEffect, useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";

// The countdown to the next reconnect attempt is redrawn this often.
const reconnectCountdownTickMs = 1_000;

export type ReconnectCountdown = {
  attempt: string;
  limit: string;
  // seconds is how long until the next attempt, or null while the engine is connecting now. A connection attempt can
  // take until the SSH timeout, so waiting and connecting are told apart rather than shown as one count.
  seconds: number | null;
};

// useReconnectCountdown says which attempt the engine's automatic reconnect is on and when the next one starts.
export function useReconnectCountdown(reconnect: TerminalSession["reconnect"]): ReconnectCountdown {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), reconnectCountdownTickMs);
    return () => window.clearInterval(timer);
  }, []);
  const retryAt = reconnect === undefined ? Number.NaN : Date.parse(reconnect.retryAt);
  const seconds = Math.ceil((retryAt - now) / 1_000);
  return {
    attempt: String(reconnect?.attempt ?? 1),
    limit: String(reconnect?.limit ?? 1),
    seconds: Number.isFinite(seconds) && seconds > 0 ? seconds : null,
  };
}

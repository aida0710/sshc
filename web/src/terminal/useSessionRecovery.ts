import { useState } from "react";

// SessionRecovery is what can be done about a session that lost its connection: reconnect an ended SSH session by
// hand, or stop the engine's automatic reconnect. The strip above the terminal and the card over it offer the same
// actions, so they share one state (a press on either shows as busy on both).
export type SessionRecovery = {
  // reconnect is undefined when this view cannot reconnect a session by hand.
  reconnect: (() => void) | undefined;
  reconnectBusy: boolean;
  reconnectFailed: boolean;
  // stopReconnecting is undefined when this view cannot stop the automatic reconnect.
  stopReconnecting: (() => void) | undefined;
  stopBusy: boolean;
};

export function useSessionRecovery({ onReconnect, onStopReconnect, onReconnected }: {
  // onReconnect resolves true once the engine reattached the SSH session.
  onReconnect: (() => Promise<boolean>) | undefined;
  onStopReconnect: (() => Promise<boolean>) | undefined;
  // onReconnected reopens the stream of the reattached session right away.
  onReconnected: () => void;
}): SessionRecovery {
  const [reconnectBusy, setReconnectBusy] = useState(false);
  const [reconnectFailed, setReconnectFailed] = useState(false);
  const [stopBusy, setStopBusy] = useState(false);

  async function reconnect(handler: () => Promise<boolean>) {
    if (reconnectBusy) return;
    setReconnectBusy(true);
    setReconnectFailed(false);
    try {
      if (await handler()) {
        onReconnected();
        return;
      }
      setReconnectFailed(true);
    } finally {
      setReconnectBusy(false);
    }
  }

  async function stopReconnecting(handler: () => Promise<boolean>) {
    if (stopBusy) return;
    setStopBusy(true);
    try {
      await handler();
    } finally {
      setStopBusy(false);
    }
  }

  return {
    reconnect: onReconnect === undefined ? undefined : () => void reconnect(onReconnect),
    reconnectBusy,
    reconnectFailed,
    stopReconnecting: onStopReconnect === undefined ? undefined : () => void stopReconnecting(onStopReconnect),
    stopBusy,
  };
}

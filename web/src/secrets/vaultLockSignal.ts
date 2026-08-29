const vaultLockChannelName = "sshc.vault-lock.v1";
const vaultLockedMessage = "locked";

export function announceVaultLocked(): void {
  if (typeof BroadcastChannel === "undefined") return;
  try {
    const channel = new BroadcastChannel(vaultLockChannelName);
    channel.postMessage(vaultLockedMessage);
    channel.close();
  } catch {
    // Polling remains the fallback when BroadcastChannel is unavailable or denied.
  }
}

export function observeVaultLocked(onLocked: () => void): () => void {
  if (typeof BroadcastChannel === "undefined") return () => undefined;
  try {
    const channel = new BroadcastChannel(vaultLockChannelName);
    const receive = (event: MessageEvent<unknown>) => {
      if (event.data === vaultLockedMessage) onLocked();
    };
    channel.addEventListener("message", receive);
    return () => {
      channel.removeEventListener("message", receive);
      channel.close();
    };
  } catch {
    return () => undefined;
  }
}

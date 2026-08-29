import { afterEach, describe, expect, it, vi } from "vitest";
import { announceVaultLocked, observeVaultLocked } from "./vaultLockSignal";

type MessageListener = (event: MessageEvent<unknown>) => void;

class FakeBroadcastChannel {
  static channels: FakeBroadcastChannel[] = [];

  readonly listeners = new Set<MessageListener>();
  closed = false;

  constructor(readonly name: string) {
    FakeBroadcastChannel.channels.push(this);
  }

  postMessage(data: unknown) {
    for (const channel of FakeBroadcastChannel.channels) {
      if (channel !== this && channel.name === this.name && !channel.closed) {
        for (const listener of channel.listeners) listener(new MessageEvent("message", { data }));
      }
    }
  }

  addEventListener(_type: "message", listener: MessageListener) {
    this.listeners.add(listener);
  }

  removeEventListener(_type: "message", listener: MessageListener) {
    this.listeners.delete(listener);
  }

  close() {
    this.closed = true;
  }
}

afterEach(() => {
  FakeBroadcastChannel.channels = [];
  vi.unstubAllGlobals();
});

describe("vault lock signal", () => {
  it("announces a lock to other tabs and releases both channels", () => {
    vi.stubGlobal("BroadcastChannel", FakeBroadcastChannel);
    const onLocked = vi.fn();
    const stop = observeVaultLocked(onLocked);

    announceVaultLocked();

    expect(onLocked).toHaveBeenCalledTimes(1);
    expect(FakeBroadcastChannel.channels).toHaveLength(2);
    expect(FakeBroadcastChannel.channels[1]?.closed).toBe(true);
    stop();
    expect(FakeBroadcastChannel.channels[0]?.closed).toBe(true);
  });

  it("remains a no-op when BroadcastChannel is unavailable", () => {
    vi.stubGlobal("BroadcastChannel", undefined);
    const onLocked = vi.fn();

    const stop = observeVaultLocked(onLocked);
    announceVaultLocked();
    stop();

    expect(onLocked).not.toHaveBeenCalled();
  });
});

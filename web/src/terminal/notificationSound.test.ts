import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  defaultNotificationSoundPreferences,
  loadNotificationSoundPreferences,
  playNotificationSound,
  primeNotificationSound,
  saveNotificationSoundPreferences,
} from "./notificationSound";

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
});

describe("notification sound preferences", () => {
  it("uses safe defaults, stores browser-local choices, and rejects unknown presets", () => {
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);
    saveNotificationSoundPreferences({ sound: "pulse", volume: 35 });
    expect(loadNotificationSoundPreferences()).toEqual({ sound: "pulse", volume: 35 });

    window.localStorage.setItem("sshc.terminal-notification-sound.v1", JSON.stringify({ sound: "remote-url", volume: 999 }));
    expect(loadNotificationSoundPreferences()).toEqual({ sound: "bell", volume: 100 });
  });

  it("falls back to the defaults and keeps working when the browser refuses storage", () => {
    window.localStorage.setItem("sshc.terminal-notification-sound.v1", "{not json");
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);

    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new DOMException("denied", "SecurityError"); });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new DOMException("denied", "SecurityError"); });
    expect(() => saveNotificationSoundPreferences({ sound: "pulse", volume: 35 })).not.toThrow();
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);
  });

  it("primes one reusable audio context from a user gesture", async () => {
    const start = vi.fn();
    const resume = vi.fn(async function (this: { state: AudioContextState }) { this.state = "running"; });
    class FakeAudioContext {
      state: AudioContextState = "suspended";
      currentTime = 0;
      destination = {};
      resume = resume;
      createGain = () => ({
        gain: { setValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() },
        connect: vi.fn(),
      });
      createOscillator = () => ({
        type: "sine" as OscillatorType,
        frequency: { value: 0 },
        connect: vi.fn(),
        start,
        stop: vi.fn(),
      });
    }
    const target = { AudioContext: FakeAudioContext } as unknown as Window;

    expect(primeNotificationSound(target)).toBe(true);
    await waitFor(() => expect(resume).toHaveBeenCalledOnce());
    expect(playNotificationSound(defaultNotificationSoundPreferences, target)).toBe(true);
    expect(start).toHaveBeenCalledTimes(2);
    expect(playNotificationSound({ sound: "none", volume: 60 }, target)).toBe(false);
  });
});

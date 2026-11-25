import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  defaultNotificationSoundPreferences,
  loadNotificationSoundPreferences,
  playNotificationSound,
  primeNotificationSound,
  withNotificationSound,
} from "./notificationSound";
import { fakeSettingsServer } from "../testing/settingsServer";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("notification sound preferences", () => {
  it("uses safe defaults until the preset in use keeps a sound", () => {
    fakeSettingsServer();
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);

    fakeSettingsServer({ presets: [{ id: "default", interface: { notificationSound: { sound: "pulse", volume: 35 } } }] });
    expect(loadNotificationSoundPreferences()).toEqual({ sound: "pulse", volume: 35 });
  });

  it("keeps a chosen sound in the preset, and leaves the default one out", () => {
    expect(withNotificationSound({ keepScreenAwake: true }, { sound: "pulse", volume: 35 }))
      .toEqual({ keepScreenAwake: true, notificationSound: { sound: "pulse", volume: 35 } });
    expect(withNotificationSound({ notificationSound: { sound: "pulse", volume: 35 } }, defaultNotificationSoundPreferences).notificationSound).toBeUndefined();
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

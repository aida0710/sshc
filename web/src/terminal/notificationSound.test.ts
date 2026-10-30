import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  defaultNotificationSoundPreferences,
  loadNotificationSoundPreferences,
  playNotificationSound,
  primeNotificationSound,
  saveNotificationSoundPreferences,
} from "./notificationSound";
import { fakeSettingsServer } from "../testing/settingsServer";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("notification sound preferences", () => {
  it("uses safe defaults and keeps a chosen sound in the preset in use", async () => {
    const server = fakeSettingsServer();
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);

    await saveNotificationSoundPreferences({ sound: "pulse", volume: 35 });

    expect(loadNotificationSoundPreferences()).toEqual({ sound: "pulse", volume: 35 });
    expect(server.document().presets[0]).toEqual({ id: "default", interface: { notificationSound: { sound: "pulse", volume: 35 } } });
  });

  it("does not write the default sound into the preset", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default", interface: { notificationSound: { sound: "pulse", volume: 35 } } }] });

    await saveNotificationSoundPreferences(defaultNotificationSoundPreferences);

    expect(server.document().presets[0]).toEqual({ id: "default" });
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

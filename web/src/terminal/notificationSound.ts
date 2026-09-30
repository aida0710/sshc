import { readStoredJSON, writeStoredJSON } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";

// The sound this browser plays when a terminal asks for attention while sshc
// is out of sight. The choice stays in this browser and never reaches the
// engine; the tones are synthesised, so nothing is fetched to play them.

export const notificationSoundPresets = ["none", "gentle", "bell", "pulse"] as const;
export type NotificationSoundPreset = typeof notificationSoundPresets[number];
export type NotificationSoundPreferences = {
  sound: NotificationSoundPreset;
  volume: number;
};

const audioContexts = new WeakMap<Window, AudioContext>();
export const defaultNotificationSoundPreferences: NotificationSoundPreferences = {
  sound: "bell",
  volume: 60,
};

function isSoundPreset(value: unknown): value is NotificationSoundPreset {
  return typeof value === "string" && (notificationSoundPresets as readonly string[]).includes(value);
}

export function loadNotificationSoundPreferences(): NotificationSoundPreferences {
  const stored = readStoredJSON(localStorageKeys.notificationSound);
  if (stored === null || typeof stored !== "object") return defaultNotificationSoundPreferences;
  const candidate = stored as Partial<NotificationSoundPreferences>;
  const volume = typeof candidate.volume === "number" && Number.isFinite(candidate.volume)
    ? Math.max(0, Math.min(100, Math.round(candidate.volume)))
    : defaultNotificationSoundPreferences.volume;
  return {
    sound: isSoundPreset(candidate.sound) ? candidate.sound : defaultNotificationSoundPreferences.sound,
    volume,
  };
}

export function saveNotificationSoundPreferences(preferences: NotificationSoundPreferences): void {
  writeStoredJSON(localStorageKeys.notificationSound, preferences);
}

function notificationAudioContext(target: Window): AudioContext | null {
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

// Browsers start an AudioContext suspended until a user gesture resumes it,
// so the first key press or click prepares the context a later sound needs.
export function primeNotificationSound(target: Window = window): boolean {
  const audio = notificationAudioContext(target);
  if (audio === null) return false;
  if (audio.state === "suspended") void audio.resume().catch(() => undefined);
  return true;
}

export function playNotificationSound(
  preferences: NotificationSoundPreferences = loadNotificationSoundPreferences(),
  target: Window = window,
): boolean {
  const preset = preferences.sound;
  if (preset === "none" || preferences.volume <= 0) return false;
  const audio = notificationAudioContext(target);
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

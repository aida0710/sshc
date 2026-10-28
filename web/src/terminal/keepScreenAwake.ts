import { localStorageKeys } from "../ui/browserStorageKeys";
import { createStoredChoice, useStoredChoice } from "../ui/storedChoice";

// Whether this browser keeps the screen on while a terminal is shown. One choice
// covers every terminal. It is about the device in front of the user (a phone
// left on the desk to watch a build), so it stays in this browser and is shared
// by its tabs, never by other devices.
const keepScreenAwake = createStoredChoice({ key: localStorageKeys.keepScreenAwake, choices: ["false", "true"], fallback: "false" });

export function loadKeepScreenAwake(): boolean {
  return keepScreenAwake.load() === "true";
}

export function saveKeepScreenAwake(enabled: boolean): void {
  keepScreenAwake.save(enabled ? "true" : "false");
}

export function useKeepScreenAwake(): boolean {
  return useStoredChoice(keepScreenAwake) === "true";
}

// The Screen Wake Lock API exists only on a secure origin (HTTPS or localhost),
// so a page opened by plain HTTP on a LAN address cannot offer the choice.
export function isScreenWakeLockSupported(): boolean {
  return "wakeLock" in navigator;
}

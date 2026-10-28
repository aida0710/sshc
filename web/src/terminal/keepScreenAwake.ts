import { useSyncExternalStore } from "react";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";

// Whether this browser keeps the screen on while a terminal is shown. One choice
// covers every terminal. It is about the device in front of the user (a phone
// left on the desk to watch a build), so it stays in this browser and is shared
// by its tabs, never by other devices.

// "storage" reaches only the other tabs, so this tab tells itself with its own event.
const changedEvent = "sshc:keep-screen-awake-changed";

export function loadKeepScreenAwake(): boolean {
  return readStoredValue(localStorageKeys.keepScreenAwake) === "true";
}

export function saveKeepScreenAwake(enabled: boolean): void {
  writeStoredValue(localStorageKeys.keepScreenAwake, String(enabled));
  window.dispatchEvent(new Event(changedEvent));
}

function subscribe(listener: () => void): () => void {
  window.addEventListener("storage", listener);
  window.addEventListener(changedEvent, listener);
  return () => { window.removeEventListener("storage", listener); window.removeEventListener(changedEvent, listener); };
}

export function useKeepScreenAwake(): boolean {
  return useSyncExternalStore(subscribe, loadKeepScreenAwake, () => false);
}

// The Screen Wake Lock API exists only on a secure origin (HTTPS or localhost),
// so a page opened by plain HTTP on a LAN address cannot offer the choice.
export function isScreenWakeLockSupported(): boolean {
  return "wakeLock" in navigator;
}

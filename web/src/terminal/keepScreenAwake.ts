import { updatePresetInterface, usePresetInterface } from "../settings/presetInterface";

// Whether the screen stays on while a terminal is shown. One choice covers every
// terminal. It is about the device in front of the user (a phone left on the
// desk to watch a build), so it is kept in the preset this machine uses.
export function saveKeepScreenAwake(enabled: boolean): void {
  void updatePresetInterface((settings) => ({ ...settings, keepScreenAwake: enabled })).catch(() => undefined);
}

export function useKeepScreenAwake(): boolean {
  return usePresetInterface().keepScreenAwake ?? false;
}

// The Screen Wake Lock API exists only on a secure origin (HTTPS or localhost),
// so a page opened by plain HTTP on a LAN address cannot offer the choice.
export function isScreenWakeLockSupported(): boolean {
  return "wakeLock" in navigator;
}

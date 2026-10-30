import { updatePresetInterface, usePresetInterface } from "../settings/presetInterface";
import { useMediaQuery } from "../ui/useMediaQuery";

// How terminals take touch. A touch terminal lays a layer of plain text over
// the terminal for the native long-press selection, opens a link on a tap, and
// raises the software keyboard only when the terminal itself is tapped. That
// layer also takes the mouse's drag selection, link clicks, wheel and
// right-click paste, so a mouse terminal leaves the terminal to xterm.js.
//
// "auto" follows what the browser says its main pointer is. A 2-in-1 PC can
// say touch while its keyboard and touchpad are attached, so the user can fix
// either one. The choice is about the device in front of the user, so it is
// kept in the preset this machine uses.
export type TouchMode = "auto" | "touch" | "mouse";
export type FixedTouchMode = Exclude<TouchMode, "auto">;

export const touchFirstQuery = "(hover: none) and (pointer: coarse)";

// saveTouchMode keeps the choice in a preset (the one this machine uses when presetId is left out); "auto" is
// the default and is not written.
export function saveTouchMode(mode: TouchMode, presetId?: string): void {
  void updatePresetInterface((settings) => ({ ...settings, touchMode: mode === "auto" ? undefined : mode }), presetId).catch(() => undefined);
}

export function useTouchMode(presetId?: string): TouchMode {
  return usePresetInterface(presetId).touchMode ?? "auto";
}

// What "auto" picks now. The browser answers again when a 2-in-1 PC changes shape.
export function useDetectedTouchMode(): FixedTouchMode {
  return useMediaQuery(touchFirstQuery) ? "touch" : "mouse";
}

export function useTouchTerminal(): boolean {
  const mode = useTouchMode();
  const detected = useDetectedTouchMode();
  return (mode === "auto" ? detected : mode) === "touch";
}

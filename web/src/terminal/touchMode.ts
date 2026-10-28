import { localStorageKeys } from "../ui/browserStorageKeys";
import { createStoredChoice, useStoredChoice } from "../ui/storedChoice";
import { useMediaQuery } from "../ui/useMediaQuery";

// How terminals take touch. A touch terminal lays a layer of plain text over
// the terminal for the native long-press selection, opens a link on a tap, and
// raises the software keyboard only when the terminal itself is tapped. That
// layer also takes the mouse's drag selection, link clicks, wheel and
// right-click paste, so a mouse terminal leaves the terminal to xterm.js.
//
// "auto" follows what the browser says its main pointer is. A 2-in-1 PC can
// say touch while its keyboard and touchpad are attached, so the user can fix
// either one. The choice is about the device in front of the user, so it stays
// in this browser.
export type TouchMode = "auto" | "touch" | "mouse";
export type FixedTouchMode = Exclude<TouchMode, "auto">;

export const touchFirstQuery = "(hover: none) and (pointer: coarse)";

const touchMode = createStoredChoice<TouchMode>({
  key: localStorageKeys.touchMode,
  choices: ["auto", "touch", "mouse"],
  fallback: "auto",
});

export const loadTouchMode = touchMode.load;
export const saveTouchMode = touchMode.save;

export function useTouchMode(): TouchMode {
  return useStoredChoice(touchMode);
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

import { createContext, useContext, useLayoutEffect, type ReactNode } from "react";
import { presetInUse, useSettings, type SettingsPreset } from "../settings/settingsStore";
import { reducedMotionQuery } from "../ui/reducedMotion";
import { useMediaQuery } from "../ui/useMediaQuery";

// ReduceMotionChoice is what a preset says about animation: follow the OS, or leave animation out ("on") or
// keep it ("off") whatever the OS says.
export type ReduceMotionChoice = "system" | "on" | "off";

export const reduceMotionChoices: readonly ReduceMotionChoice[] = ["system", "on", "off"];

// reduceMotionChoiceOf reads the choice a preset keeps. The preset leaves it out to follow the OS.
export function reduceMotionChoiceOf(preset: SettingsPreset): ReduceMotionChoice {
  const stored = preset.appearance?.reduceMotion;
  if (stored === undefined) return "system";
  return stored ? "on" : "off";
}

// withReduceMotion is the preset keeping choice. Following the OS is kept as no value, the same as a preset
// saved before the setting existed.
export function withReduceMotion(preset: SettingsPreset, choice: ReduceMotionChoice): SettingsPreset {
  const appearance = { ...preset.appearance };
  delete appearance.reduceMotion;
  if (choice !== "system") appearance.reduceMotion = choice === "on";
  return { ...preset, appearance };
}

function isMotionReduced(choice: ReduceMotionChoice, systemReducesMotion: boolean): boolean {
  if (choice === "system") return systemReducesMotion;
  return choice === "on";
}

const ReducedMotionContext = createContext<boolean | null>(null);

// MotionProvider keeps <html data-motion="reduce"> while animation is left out, for index.css and its
// motion-reduce variant, and gives the same answer to components that animate by script (useReducedMotion).
// Until the synced settings arrive (lock screen, first paint) it follows the OS.
export function MotionProvider({ children }: { children: ReactNode }) {
  const choice = reduceMotionChoiceOf(presetInUse(useSettings().document));
  const systemReducesMotion = useMediaQuery(reducedMotionQuery);
  const reduced = isMotionReduced(choice, systemReducesMotion);

  // A layout effect, so the attribute is on <html> before the first paint that could start an animation.
  useLayoutEffect(() => {
    const root = document.documentElement;
    if (reduced) root.dataset.motion = "reduce";
    else delete root.dataset.motion;
  }, [reduced]);

  return <ReducedMotionContext.Provider value={reduced}>{children}</ReducedMotionContext.Provider>;
}

// useReducedMotion is whether to leave animation out. Outside MotionProvider (a component tested alone) it
// follows the OS.
export function useReducedMotion(): boolean {
  const provided = useContext(ReducedMotionContext);
  const systemReducesMotion = useMediaQuery(reducedMotionQuery);
  return provided ?? systemReducesMotion;
}

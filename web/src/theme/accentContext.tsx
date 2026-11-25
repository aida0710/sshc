import { createContext, useContext, useEffect, useMemo, type ReactNode } from "react";
import { presetInUse, useSettings, type SettingsPreset } from "../settings/settingsStore";
import { applyAccent, defaultAccentChoice, isAccent, type AccentChoice } from "./accent";
import { isAccentColor } from "./accentColor";

const AccentContext = createContext<AccentChoice | null>(null);

// accentChoiceOf reads the one accent a preset keeps: a preset colour's name or a colour of one's own.
export function accentChoiceOf(preset: SettingsPreset): AccentChoice {
  const accent = preset.appearance?.accent;
  if (isAccentColor(accent)) return { accent: defaultAccentChoice.accent, customColor: accent };
  return { accent: isAccent(accent) ? accent : defaultAccentChoice.accent, customColor: null };
}

// AccentProvider keeps <html data-accent> on the accent of the preset this
// machine uses, next to ThemeProvider which does the same for the theme.
// Before the settings arrive (lock screen, first paint) the default accent shows.
export function AccentProvider({ children }: { children: ReactNode }) {
  const settings = useSettings();
  const choice = useMemo(
    () => settings.document === null ? defaultAccentChoice : accentChoiceOf(presetInUse(settings.document)),
    [settings.document],
  );

  useEffect(() => {
    applyAccent(document.documentElement, choice);
  }, [choice]);

  return <AccentContext.Provider value={choice}>{children}</AccentContext.Provider>;
}

export function useAccent(): AccentChoice {
  return useContext(AccentContext) ?? defaultAccentChoice;
}

// withAccent is preset keeping the accent chosen: a colour of one's own when there is one, or the preset colour.
export function withAccent(preset: SettingsPreset, choice: AccentChoice): SettingsPreset {
  return { ...preset, appearance: { ...preset.appearance, accent: choice.customColor ?? choice.accent } };
}

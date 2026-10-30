import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { presetInUse, updatePresetInUse, useSettings, type SettingsPreset } from "../settings/settingsStore";
import { applyAccent, defaultAccentChoice, isAccent, type Accent, type AccentChoice } from "./accent";
import { isAccentColor } from "./accentColor";

type AccentState = AccentChoice & {
  // chooseAccent shows the preset at once and saves it, giving up a colour of
  // one's own. When saving fails the saved colour comes back and it rejects,
  // so the caller can say why.
  chooseAccent: (accent: Accent) => Promise<void>;
  // previewCustomColor shows a custom colour without saving it, while the
  // colour picker is still being dragged.
  previewCustomColor: (color: string) => void;
  // chooseCustomColor shows the custom accent in this colour and saves it,
  // like chooseAccent.
  chooseCustomColor: (color: string) => Promise<void>;
};

const AccentContext = createContext<AccentState | null>(null);

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
  const synced = useMemo(
    () => settings.document === null ? defaultAccentChoice : accentChoiceOf(presetInUse(settings.document)),
    [settings.document],
  );
  // preview is a colour on screen that is not saved yet: one being dragged in the picker.
  const [preview, setPreview] = useState<AccentChoice | null>(null);
  const choice = preview ?? synced;

  useEffect(() => {
    applyAccent(document.documentElement, choice);
  }, [choice]);

  const save = useCallback(async (next: AccentChoice) => {
    setPreview(null);
    await updatePresetInUse((preset) => withAccent(preset, next));
  }, []);

  const chooseAccent = useCallback((accent: Accent) => save({ accent, customColor: null }), [save]);
  // The preset under a colour of one's own is the one chosen last.
  const chooseCustomColor = useCallback((color: string) => save({ accent: synced.accent, customColor: color }), [save, synced.accent]);
  const previewCustomColor = useCallback((color: string) => {
    setPreview({ accent: synced.accent, customColor: color });
  }, [synced.accent]);

  const value = useMemo<AccentState>(
    () => ({ ...choice, chooseAccent, previewCustomColor, chooseCustomColor }),
    [choice, chooseAccent, previewCustomColor, chooseCustomColor],
  );
  return <AccentContext.Provider value={value}>{children}</AccentContext.Provider>;
}

const outsideProvider: AccentState = {
  ...defaultAccentChoice,
  chooseAccent: () => Promise.reject(new Error("the accent can only be chosen inside AccentProvider")),
  previewCustomColor: () => undefined,
  chooseCustomColor: () => Promise.reject(new Error("the accent can only be chosen inside AccentProvider")),
};

export function useAccent(): AccentState {
  return useContext(AccentContext) ?? outsideProvider;
}

// withAccent is preset keeping the accent chosen: a colour of one's own when there is one, or the preset colour.
export function withAccent(preset: SettingsPreset, choice: AccentChoice): SettingsPreset {
  return { ...preset, appearance: { ...preset.appearance, accent: choice.customColor ?? choice.accent } };
}

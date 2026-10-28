import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { AppearanceApi, AppearanceSettings } from "../api/appearance";
import { applyAccent, defaultAccentChoice, isAccent, type Accent, type AccentChoice } from "./accent";
import { isAccentColor } from "./accentColor";

type AccentState = AccentChoice & {
  // loadAccent reads the saved accent. The app calls it once the engine is
  // ready; before that (lock screen, first paint) the default accent shows.
  loadAccent: () => void;
  // chooseAccent shows the preset at once and saves it, giving up a colour of
  // one's own. When saving fails it puts the previous colour back and rejects,
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

// The engine keeps one accent: a preset's name or a colour of one's own.
function choiceFrom(settings: AppearanceSettings): AccentChoice {
  if (isAccentColor(settings.accent)) return { accent: defaultAccentChoice.accent, customColor: settings.accent };
  return { accent: isAccent(settings.accent) ? settings.accent : defaultAccentChoice.accent, customColor: null };
}

function settingsFrom(choice: AccentChoice): AppearanceSettings {
  return { accent: choice.customColor ?? choice.accent };
}

// AccentProvider keeps <html data-accent> on the accent saved in this
// machine's metadata, next to ThemeProvider which does the same for the theme.
export function AccentProvider({ api, children }: { api: AppearanceApi; children: ReactNode }) {
  const [choice, setChoice] = useState<AccentChoice>(defaultAccentChoice);
  // saved is the last choice the engine holds, put back when a save fails.
  const saved = useRef<AccentChoice>(defaultAccentChoice);
  // A colour the user chose wins over a read that was still on its way.
  const chosen = useRef(false);

  useEffect(() => {
    applyAccent(document.documentElement, choice);
  }, [choice]);

  const loadAccent = useCallback(() => {
    chosen.current = false;
    api.appearanceSettings().then(
      (settings) => {
        if (chosen.current) return;
        saved.current = choiceFrom(settings);
        setChoice(saved.current);
      },
      // The colour is only decoration: keep the one on screen. Saving reports
      // its own failure on the settings page.
      () => undefined,
    );
  }, [api]);

  const save = useCallback(async (next: AccentChoice) => {
    const previous = saved.current;
    chosen.current = true;
    setChoice(next);
    try {
      await api.setAppearanceSettings(settingsFrom(next));
      saved.current = next;
    } catch (error) {
      setChoice(previous);
      throw error;
    }
  }, [api]);

  const chooseAccent = useCallback((accent: Accent) => save({ accent, customColor: null }), [save]);
  // The preset under a colour of one's own is the one chosen last, for an older sshc to show.
  const chooseCustomColor = useCallback((color: string) => save({ accent: saved.current.accent, customColor: color }), [save]);
  const previewCustomColor = useCallback((color: string) => {
    chosen.current = true;
    setChoice({ accent: saved.current.accent, customColor: color });
  }, []);

  const value = useMemo<AccentState>(
    () => ({ ...choice, loadAccent, chooseAccent, previewCustomColor, chooseCustomColor }),
    [choice, loadAccent, chooseAccent, previewCustomColor, chooseCustomColor],
  );
  return <AccentContext.Provider value={value}>{children}</AccentContext.Provider>;
}

const outsideProvider: AccentState = {
  ...defaultAccentChoice,
  loadAccent: () => undefined,
  chooseAccent: () => Promise.reject(new Error("the accent can only be chosen inside AccentProvider")),
  previewCustomColor: () => undefined,
  chooseCustomColor: () => Promise.reject(new Error("the accent can only be chosen inside AccentProvider")),
};

export function useAccent(): AccentState {
  return useContext(AccentContext) ?? outsideProvider;
}

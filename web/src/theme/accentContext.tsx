import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { AppearanceApi } from "../api/appearance";
import { applyAccent, defaultAccent, isAccent, type Accent } from "./accent";

type AccentState = {
  accent: Accent;
  // loadAccent reads the saved accent. The app calls it once the engine is
  // ready; before that (lock screen, first paint) the default accent shows.
  loadAccent: () => void;
  // chooseAccent shows the colour at once and saves it. When saving fails it
  // puts the previous colour back and rejects, so the caller can say why.
  chooseAccent: (accent: Accent) => Promise<void>;
};

const AccentContext = createContext<AccentState | null>(null);

// AccentProvider keeps <html data-accent> on the accent saved in this
// machine's metadata, next to ThemeProvider which does the same for the theme.
export function AccentProvider({ api, children }: { api: AppearanceApi; children: ReactNode }) {
  const [accent, setAccent] = useState<Accent>(defaultAccent);
  const current = useRef<Accent>(defaultAccent);
  // A colour the user chose wins over a read that was still on its way.
  const chosen = useRef(false);

  const show = useCallback((next: Accent) => {
    current.current = next;
    setAccent(next);
  }, []);

  useEffect(() => {
    applyAccent(document.documentElement, accent);
  }, [accent]);

  const loadAccent = useCallback(() => {
    chosen.current = false;
    api.appearanceSettings().then(
      (settings) => {
        if (!chosen.current) show(isAccent(settings.accent) ? settings.accent : defaultAccent);
      },
      // The colour is only decoration: keep the one on screen. Saving reports
      // its own failure on the settings page.
      () => undefined,
    );
  }, [api, show]);

  const chooseAccent = useCallback(async (next: Accent) => {
    const previous = current.current;
    chosen.current = true;
    show(next);
    try {
      await api.setAppearanceSettings({ accent: next });
    } catch (error) {
      show(previous);
      throw error;
    }
  }, [api, show]);

  const value = useMemo<AccentState>(() => ({ accent, loadAccent, chooseAccent }), [accent, loadAccent, chooseAccent]);
  return <AccentContext.Provider value={value}>{children}</AccentContext.Provider>;
}

const outsideProvider: AccentState = {
  accent: defaultAccent,
  loadAccent: () => undefined,
  chooseAccent: () => Promise.reject(new Error("the accent can only be chosen inside AccentProvider")),
};

export function useAccent(): AccentState {
  return useContext(AccentContext) ?? outsideProvider;
}

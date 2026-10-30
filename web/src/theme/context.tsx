import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useState, type ReactNode } from "react";
import { presetInUse, updatePresetInUse, useSettings } from "../settings/settingsStore";
import { applyTheme, copyTheme, detectTheme, isTheme, resolveTheme, schemeOf, type ColorScheme, type ColorTheme, type Theme } from "./theme";

type ThemeState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  // resolved is the theme on screen; scheme is whether it is light or dark.
  resolved: ColorTheme;
  scheme: ColorScheme;
};

const ThemeContext = createContext<ThemeState | null>(null);

const darkQuery = "(prefers-color-scheme: dark)";

function systemPrefersDark(): boolean {
  if (typeof window.matchMedia !== "function") return false;
  return window.matchMedia(darkQuery).matches;
}

// ThemeProvider draws with the copied theme until the synced settings arrive, then with the theme of the
// preset this machine uses. While the preset chooses none, the theme on screen stays, so one picked before
// unlocking, or kept in this browser before settings were synced, is not replaced by the default. A test that
// passes initial keeps that theme.
export function ThemeProvider({ children, initial }: { children: ReactNode; initial?: Theme }) {
  const [theme, setThemeState] = useState<Theme>(() => initial ?? detectTheme());
  const [prefersDark, setPrefersDark] = useState<boolean>(() => systemPrefersDark());
  const chosen = presetInUse(useSettings().document).appearance?.theme;
  const synced = isTheme(chosen) ? chosen : null;

  useEffect(() => {
    if (initial !== undefined || synced === null) return;
    setThemeState(synced);
    copyTheme(synced);
  }, [initial, synced]);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia(darkQuery);
    const update = () => setPrefersDark(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  const resolved = resolveTheme(theme, prefersDark);

  // A layout effect, so the theme is on <html> before any component's effect reads its colours from the
  // stylesheet (the terminal, Android's system bars). A child's effects run before its parent's, so a plain
  // effect here would come after theirs and they would read the previous theme.
  useLayoutEffect(() => {
    applyTheme(document.documentElement, resolved);
  }, [resolved]);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    copyTheme(next);
    // A refused save leaves the theme on screen; the next read of the settings puts the synced one back.
    void updatePresetInUse((preset) => ({ ...preset, appearance: { ...preset.appearance, theme: next } })).catch(() => undefined);
  }, []);

  const value = useMemo<ThemeState>(
    () => ({ theme, setTheme, resolved, scheme: schemeOf(resolved) }),
    [theme, setTheme, resolved],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

const fallback: ThemeState = { theme: "dark", setTheme: () => undefined, resolved: "dark", scheme: "dark" };

export function useTheme(): ThemeState {
  return useContext(ThemeContext) ?? fallback;
}

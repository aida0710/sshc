import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { applyTheme, detectTheme, rememberTheme, resolveTheme, type Theme } from "./theme";

type ThemeState = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  resolved: "light" | "dark";
};

const ThemeContext = createContext<ThemeState | null>(null);

const darkQuery = "(prefers-color-scheme: dark)";

function systemPrefersDark(): boolean {
  if (typeof window.matchMedia !== "function") return false;
  return window.matchMedia(darkQuery).matches;
}

export function ThemeProvider({ children, initial }: { children: ReactNode; initial?: Theme }) {
  const [theme, setThemeState] = useState<Theme>(() => initial ?? detectTheme());
  const [prefersDark, setPrefersDark] = useState<boolean>(() => systemPrefersDark());

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia(darkQuery);
    const update = () => setPrefersDark(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  const resolved = resolveTheme(theme, prefersDark);

  useEffect(() => {
    applyTheme(document.documentElement, resolved);
  }, [resolved]);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    rememberTheme(next);
  }, []);

  const value = useMemo<ThemeState>(() => ({ theme, setTheme, resolved }), [theme, setTheme, resolved]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

const fallback: ThemeState = { theme: "system", setTheme: () => undefined, resolved: "light" };

export function useTheme(): ThemeState {
  return useContext(ThemeContext) ?? fallback;
}

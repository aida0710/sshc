import { readStoredValue, writeStoredValue } from "../ui/browserStorage";

export const themes = ["system", "light", "dark"] as const;
export type Theme = (typeof themes)[number];

export const defaultTheme: Theme = "dark";

export const themeStorageKey = "sshc.theme";

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && (themes as readonly string[]).includes(value);
}

export function detectTheme(): Theme {
  const stored = readStoredValue(themeStorageKey);
  return isTheme(stored) ? stored : defaultTheme;
}

export function rememberTheme(theme: Theme): void {
  writeStoredValue(themeStorageKey, theme);
}

export function resolveTheme(theme: Theme, systemPrefersDark: boolean): "light" | "dark" {
  if (theme === "system") return systemPrefersDark ? "dark" : "light";
  return theme;
}

export function applyTheme(root: HTMLElement, resolved: "light" | "dark"): void {
  root.setAttribute("data-theme", resolved);
}

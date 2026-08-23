export const themes = ["system", "light", "dark"] as const;
export type Theme = (typeof themes)[number];

export const defaultTheme: Theme = "system";

export const themeStorageKey = "sshc.theme";

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && (themes as readonly string[]).includes(value);
}

export function detectTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(themeStorageKey);
    if (isTheme(stored)) return stored;
  } catch {
  }
  return defaultTheme;
}

export function rememberTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(themeStorageKey, theme);
  } catch {
  }
}

export function resolveTheme(theme: Theme, systemPrefersDark: boolean): "light" | "dark" {
  if (theme === "system") return systemPrefersDark ? "dark" : "light";
  return theme;
}

export function applyTheme(root: HTMLElement, resolved: "light" | "dark"): void {
  root.setAttribute("data-theme", resolved);
}

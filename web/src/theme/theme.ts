import type { MessageKey } from "../i18n/messages";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";

// ColorScheme is whether a theme is light or dark. The accent colours, the code editor's theme and
// Android's system bars come in these two kinds only, so a theme picks one of them by its scheme.
export type ColorScheme = "light" | "dark";

// The themes with colours of their own, in the order the settings page shows them. A theme's colours are
// the file themes/<name>.css, which index.css imports. To add a theme, add a line here, that file and its
// label in both languages; themes.test.ts checks the three agree.
export const colorThemes = [
  { name: "light", scheme: "light", label: "shell.themeLight" },
  { name: "dark", scheme: "dark", label: "shell.themeDark" },
] as const satisfies readonly { name: string; scheme: ColorScheme; label: MessageKey }[];

export type ColorTheme = (typeof colorThemes)[number]["name"];

// Theme is a choice on the settings page: a theme, or "system", which follows the system's light or dark
// with the light and dark themes.
export type Theme = "system" | ColorTheme;

export const themes: readonly Theme[] = ["system", ...colorThemes.map((theme) => theme.name)];

export const defaultTheme: Theme = "dark";

export function isTheme(value: unknown): value is Theme {
  return typeof value === "string" && (themes as readonly string[]).includes(value);
}

function colorTheme(name: ColorTheme): (typeof colorThemes)[number] {
  return colorThemes.find((theme) => theme.name === name)!;
}

export function schemeOf(theme: ColorTheme): ColorScheme {
  return colorTheme(theme).scheme;
}

export function themeLabel(theme: Theme): MessageKey {
  return theme === "system" ? "shell.themeSystem" : colorTheme(theme).label;
}

// detectTheme is the theme to draw with before the synced settings arrive: the copy kept from the last time
// they were read. The screens before sign-in and unlock only ever see this.
export function detectTheme(): Theme {
  const copied = readStoredValue(localStorageKeys.theme);
  return isTheme(copied) ? copied : defaultTheme;
}

// copyTheme keeps the theme for the next first paint. It is a copy of the synced setting, never read back into it.
export function copyTheme(theme: Theme): void {
  writeStoredValue(localStorageKeys.theme, theme);
}

export function resolveTheme(theme: Theme, systemPrefersDark: boolean): ColorTheme {
  if (theme === "system") return systemPrefersDark ? "dark" : "light";
  return theme;
}

// themeAttributes are what paints an element in a theme: data-theme picks the theme's colours and
// data-scheme the light or dark accent colours that go with them. A preview of another theme inside the page
// (the settings' thumbnails) carries both too.
export function themeAttributes(theme: ColorTheme): { "data-theme": ColorTheme; "data-scheme": ColorScheme } {
  return { "data-theme": theme, "data-scheme": schemeOf(theme) };
}

export function applyTheme(root: HTMLElement, theme: ColorTheme): void {
  for (const [name, value] of Object.entries(themeAttributes(theme))) root.setAttribute(name, value);
}

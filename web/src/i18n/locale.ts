export const locales = ["en", "ja"] as const;
export type Locale = (typeof locales)[number];

export const defaultLocale: Locale = "en";

export const storageKey = "sshc.language";

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && (locales as readonly string[]).includes(value);
}

export function detectLocale(): Locale {
  try {
    const stored = window.localStorage.getItem(storageKey);
    if (isLocale(stored)) return stored;
  } catch {
  }
  for (const candidate of navigator.languages ?? [navigator.language]) {
    const subtag = candidate.split("-")[0];
    if (isLocale(subtag)) return subtag;
  }
  return defaultLocale;
}

export function rememberLocale(locale: Locale): void {
  try {
    window.localStorage.setItem(storageKey, locale);
  } catch {
  }
}

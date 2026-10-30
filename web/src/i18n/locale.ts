import { forgetStoredValue, readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import type { MessageKey } from "./messages";

export const locales = ["en", "ja"] as const;
export type Locale = (typeof locales)[number];

export const defaultLocale: Locale = "en";

// Each language is named in itself, so a person can find their own.
export const localeLabelKeys: Record<Locale, MessageKey> = {
  en: "shell.languageEnglish",
  ja: "shell.languageJapanese",
};

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && (locales as readonly string[]).includes(value);
}

// browserLocale is the browser's own language, used while the synced settings choose none.
export function browserLocale(): Locale {
  for (const candidate of navigator.languages ?? [navigator.language]) {
    const subtag = candidate.split("-")[0];
    if (isLocale(subtag)) return subtag;
  }
  return defaultLocale;
}

// detectLocale is the language to draw with before the synced settings arrive: the copy kept from the last
// time they were read, or the browser's language. The screens before sign-in and unlock only ever see this.
export function detectLocale(): Locale {
  const copied = readStoredValue(localStorageKeys.locale);
  return isLocale(copied) ? copied : browserLocale();
}

// copyLocale keeps the language for the next first paint. It is a copy of the synced setting, never read back
// into it.
export function copyLocale(locale: Locale): void {
  writeStoredValue(localStorageKeys.locale, locale);
}

// forgetLocale drops the copy, so the next first paint follows the browser's language.
export function forgetLocale(): void {
  forgetStoredValue(localStorageKeys.locale);
}

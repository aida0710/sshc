import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import type { MessageKey } from "./messages";

export const locales = ["en", "ja"] as const;
export type Locale = (typeof locales)[number];

export const defaultLocale: Locale = "en";

export const storageKey = "sshc.language";

// Each language is named in itself, so a person can find their own.
export const localeLabelKeys: Record<Locale, MessageKey> = {
  en: "shell.languageEnglish",
  ja: "shell.languageJapanese",
};

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && (locales as readonly string[]).includes(value);
}

export function detectLocale(): Locale {
  const stored = readStoredValue(storageKey);
  if (isLocale(stored)) return stored;
  for (const candidate of navigator.languages ?? [navigator.language]) {
    const subtag = candidate.split("-")[0];
    if (isLocale(subtag)) return subtag;
  }
  return defaultLocale;
}

export function rememberLocale(locale: Locale): void {
  writeStoredValue(storageKey, locale);
}

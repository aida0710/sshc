import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { updateGlobalSettings, useSettings } from "../settings/settingsStore";
import { messages, type MessageKey } from "./messages";
import { browserLocale, copyLocale, defaultLocale, detectLocale, forgetLocale, isLocale, type Locale } from "./locale";

export type Values = Record<string, string | number>;

export type Translate = (key: MessageKey, values?: Values) => string;

type LanguageState = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  // followBrowser drops the chosen language: this browser shows its own language from then on.
  followBrowser: () => void;
  t: Translate;
};

const LanguageContext = createContext<LanguageState | null>(null);

function interpolate(template: string, values: Values | undefined): string {
  if (values === undefined) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in values ? String(values[name]) : whole,
  );
}

// LanguageProvider draws with the copied language (or the browser's) until the synced settings arrive, then
// with the language they choose. While they choose none, the language on screen stays, so one picked before
// unlocking, or kept in this browser before settings were synced, is not replaced by the browser's. A test
// that passes initial keeps that language.
export function LanguageProvider({ children, initial }: { children: ReactNode; initial?: Locale }) {
  const [locale, setLocaleState] = useState<Locale>(() => initial ?? detectLocale());
  const chosen = useSettings().document?.global.language;
  const synced = isLocale(chosen) ? chosen : null;

  useEffect(() => {
    if (initial !== undefined || synced === null) return;
    setLocaleState(synced);
    copyLocale(synced);
  }, [initial, synced]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    copyLocale(next);
    // A refused save leaves the language on screen; the next read of the settings puts the synced one back.
    void updateGlobalSettings((settings) => ({ ...settings, language: next })).catch(() => undefined);
  }, []);

  const followBrowser = useCallback(() => {
    setLocaleState(browserLocale());
    forgetLocale();
    void updateGlobalSettings(({ language: _previous, ...settings }) => settings).catch(() => undefined);
  }, []);

  const value = useMemo<LanguageState>(() => {
    const catalogue = messages[locale];
    return {
      locale,
      setLocale,
      followBrowser,
      t: (key, values) => interpolate(catalogue[key] ?? messages[defaultLocale][key], values),
    };
  }, [locale, setLocale, followBrowser]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

const fallback: LanguageState = {
  locale: defaultLocale,
  setLocale: () => undefined,
  followBrowser: () => undefined,
  t: (key, values) => interpolate(messages[defaultLocale][key], values),
};

export function useLanguage(): LanguageState {
  return useContext(LanguageContext) ?? fallback;
}

export function useTranslate(): Translate {
  return useLanguage().t;
}

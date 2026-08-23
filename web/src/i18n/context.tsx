import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { messages, type MessageKey } from "./messages";
import { defaultLocale, detectLocale, rememberLocale, type Locale } from "./locale";

export type Values = Record<string, string | number>;

export type Translate = (key: MessageKey, values?: Values) => string;

type LanguageState = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: Translate;
};

const LanguageContext = createContext<LanguageState | null>(null);

function interpolate(template: string, values: Values | undefined): string {
  if (values === undefined) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in values ? String(values[name]) : whole,
  );
}

export function LanguageProvider({ children, initial }: { children: ReactNode; initial?: Locale }) {
  const [locale, setLocaleState] = useState<Locale>(() => initial ?? detectLocale());

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    rememberLocale(next);
  }, []);

  const value = useMemo<LanguageState>(() => {
    const catalogue = messages[locale];
    return {
      locale,
      setLocale,
      t: (key, values) => interpolate(catalogue[key] ?? messages[defaultLocale][key], values),
    };
  }, [locale, setLocale]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

const fallback: LanguageState = {
  locale: defaultLocale,
  setLocale: () => undefined,
  t: (key, values) => interpolate(messages[defaultLocale][key], values),
};

export function useLanguage(): LanguageState {
  return useContext(LanguageContext) ?? fallback;
}

export function useTranslate(): Translate {
  return useLanguage().t;
}

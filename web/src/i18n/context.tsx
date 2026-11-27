import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { updateGlobalSettings, useSettings } from "../settings/settingsStore";
import { runsInAndroidApp } from "../android/native";
import { androidCatalogue } from "./androidCatalogue";
import { messages, type MessageKey } from "./messages";
import { browserLocale, copyLocale, defaultLocale, detectLocale, forgetLocale, isLocale, type Locale } from "./locale";

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

// LanguageProvider draws with the copied language (or the browser's) until the synced settings arrive, then
// with the language they choose. While they choose none, the language on screen stays, so one picked before
// unlocking, or kept in this browser before settings were synced, is not replaced by the browser's. A test
// that passes initial keeps that language.
export function LanguageProvider({ children, initial }: { children: ReactNode; initial?: Locale }) {
  const [locale, setLocaleState] = useState<Locale>(() => initial ?? detectLocale());
  const settings = useSettings().document;
  const settingsRead = settings !== null;
  const chosen = settings?.global.language;
  const synced = isLocale(chosen) ? chosen : null;

  useEffect(() => {
    if (initial !== undefined || synced === null) return;
    setLocaleState(synced);
    copyLocale(synced);
  }, [initial, synced]);

  // readFor は、言語を写した設定のエンジン（machineId）。リモートセッションを替えた先の設定が言語を選んでいなければ、
  // 前のエンジンの言語を残さず、ブラウザの言語にする。
  const machineId = settings?.machineId ?? null;
  const readFor = useRef<string | null>(null);
  useEffect(() => {
    if (initial !== undefined || machineId === null) return;
    const anotherEngine = readFor.current !== null && readFor.current !== machineId;
    readFor.current = machineId;
    if (!anotherEngine || synced !== null) return;
    setLocaleState(browserLocale());
    forgetLocale();
  }, [initial, machineId, synced]);

  // A language the settings chose and then dropped (the settings page, here or on another machine) means following
  // the browser from then on.
  const chosenBefore = useRef(synced);
  useEffect(() => {
    const dropped = chosenBefore.current !== null && synced === null && settingsRead;
    chosenBefore.current = synced;
    if (initial !== undefined || !dropped) return;
    setLocaleState(browserLocale());
    forgetLocale();
  }, [initial, settingsRead, synced]);

  // The browser chooses fallback fonts, line breaks and the screen reader's voice from this. A layout
  // effect sets it before the first paint, so the first frame is not drawn with the fonts of another language.
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    copyLocale(next);
    // A refused save leaves the language on screen; the next read of the settings puts the synced one back.
    void updateGlobalSettings((settings) => ({ ...settings, language: next })).catch(() => undefined);
  }, []);

  const value = useMemo<LanguageState>(() => {
    const catalogue = runsInAndroidApp() ? androidCatalogue(locale) : messages[locale];
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

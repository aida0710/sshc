import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { LanguageProvider, useTranslate, type Translate } from "../i18n/context";
import type { Locale } from "../i18n/locale";

// translatorFor is the app's own Translate in that language, for testing a
// function that takes one outside a component.
export function translatorFor(locale: Locale): Translate {
  const { result } = renderHook(() => useTranslate(), {
    wrapper: ({ children }: { children: ReactNode }) => createElement(LanguageProvider, { initial: locale, children }),
  });
  return result.current;
}

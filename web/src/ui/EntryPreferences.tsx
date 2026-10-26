import { useLanguage } from "../i18n/context";
import { localeLabelKeys, locales, type Locale } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { useTheme } from "../theme/context";
import { themes, type Theme } from "../theme/theme";
import { ChevronSelect } from "./ChevronSelect";
import type { IconName } from "./icons";

const themeLabels: Record<Theme, MessageKey> = {
  system: "shell.themeSystem",
  light: "shell.themeLight",
  dark: "shell.themeDark",
};

const themeIcons: Record<Theme, IconName> = {
  system: "themeSystem",
  light: "themeLight",
  dark: "themeDark",
};

// EntryPreferences are the theme and language choices in the corner of a screen shown before the app
// opens, so they can be changed before signing in.
export function EntryPreferences() {
  const { t, locale, setLocale } = useLanguage();
  const { theme, setTheme } = useTheme();
  return (
    <>
      <ChevronSelect aria-label={t("shell.themeMenu")} leadingIcon={themeIcons[theme]} value={theme}
        onChange={(event) => setTheme(event.target.value as Theme)} className="bg-sidebar">
        {themes.map((candidate) => <option key={candidate} value={candidate}>{t(themeLabels[candidate])}</option>)}
      </ChevronSelect>
      <ChevronSelect aria-label={t("shell.languageMenu")} leadingIcon="language" value={locale}
        onChange={(event) => setLocale(event.target.value as Locale)} className="bg-sidebar">
        {locales.map((candidate) => <option key={candidate} value={candidate}>{t(localeLabelKeys[candidate])}</option>)}
      </ChevronSelect>
    </>
  );
}

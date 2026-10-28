import { useLanguage } from "../i18n/context";
import { localeLabelKeys, locales, type Locale } from "../i18n/locale";
import { useTheme } from "../theme/context";
import { schemeOf, themeLabel, themes, type Theme } from "../theme/theme";
import { ChevronSelect } from "./ChevronSelect";
import type { IconName } from "./icons";

// A theme's icon is the sun or the moon of its scheme, so a new theme needs no icon of its own.
function themeIcon(theme: Theme): IconName {
  if (theme === "system") return "themeSystem";
  return schemeOf(theme) === "light" ? "themeLight" : "themeDark";
}

// EntryPreferences are the theme and language choices in the corner of a screen shown before the app
// opens, so they can be changed before signing in.
export function EntryPreferences() {
  const { t, locale, setLocale } = useLanguage();
  const { theme, setTheme } = useTheme();
  return (
    <>
      <ChevronSelect aria-label={t("shell.themeMenu")} leadingIcon={themeIcon(theme)} value={theme}
        onChange={(event) => setTheme(event.target.value as Theme)} className="bg-sidebar">
        {themes.map((candidate) => <option key={candidate} value={candidate}>{t(themeLabel(candidate))}</option>)}
      </ChevronSelect>
      <ChevronSelect aria-label={t("shell.languageMenu")} leadingIcon="language" value={locale}
        onChange={(event) => setLocale(event.target.value as Locale)} className="bg-sidebar">
        {locales.map((candidate) => <option key={candidate} value={candidate}>{t(localeLabelKeys[candidate])}</option>)}
      </ChevronSelect>
    </>
  );
}

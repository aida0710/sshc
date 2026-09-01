import type { MouseEvent } from "react";
import { useLanguage } from "../i18n/context";
import { locales, type Locale } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { useTheme } from "../theme/context";
import { themes, type Theme } from "../theme/theme";
import { autoControl } from "../ui/form";
import { Icon, type IconName } from "../ui/icons";

export type MenuItem = {
  key: string;
  label: MessageKey;
  icon: IconName;
  href: string;
};

export type MenuGroup = {
  label: MessageKey;
  items: MenuItem[];
};

const themeLabels: Record<Theme, MessageKey> = {
  system: "shell.themeSystem",
  light: "shell.themeLight",
  dark: "shell.themeDark",
};

const localeLabels: Record<Locale, MessageKey> = {
  en: "shell.languageEnglish",
  ja: "shell.languageJapanese",
};

export function MenuPanel({
  groups,
  onNavigate,
}: {
  groups: MenuGroup[];
  onNavigate: (href: string) => void;
}) {
  const { t, locale, setLocale } = useLanguage();
  const { theme, setTheme } = useTheme();

  function follow(event: MouseEvent<HTMLAnchorElement>, href: string) {
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    onNavigate(href);
  }

  return (
    <section aria-labelledby="menu-heading" className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="border-b border-line pb-3">
        <h2 id="menu-heading" className="text-xl font-semibold tracking-tight text-ink">
          {t("section.menu")}
        </h2>
      </div>

      <div className="grid items-start gap-x-8 gap-y-7 lg:grid-cols-2 xl:grid-cols-3">
        {groups.map((group) => (
          <section key={group.label} aria-labelledby={`menu-${group.label}`}>
            <h3
              id={`menu-${group.label}`}
              className="px-1 text-[11px] font-bold uppercase tracking-[0.14em] text-ink-faint"
            >
              {t(group.label)}
            </h3>
            <ul className="mt-2 overflow-hidden rounded border border-line bg-card">
              {group.items.map((item, index) => {
                const label = t(item.label);
                return (
                  <li key={item.key} className={index === 0 ? "" : "border-t border-line"}>
                    <a
                      href={item.href}
                      aria-label={t("menu.open", { section: label })}
                      onClick={(event) => follow(event, item.href)}
                      className="group flex min-h-14 items-center gap-3 px-3 py-2.5 transition-colors hover:bg-select-fill"
                    >
                      <span className="grid size-8 shrink-0 place-items-center rounded bg-control text-ink-muted group-hover:text-ink">
                        <Icon name={item.icon} className="size-4" />
                      </span>
                      <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">{label}</span>
                      <Icon name="chevronRight" className="size-4 shrink-0 text-ink-faint group-hover:text-ink-muted" />
                    </a>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
        <section aria-labelledby="menu-display-settings">
          <h3
            id="menu-display-settings"
            className="px-1 text-[11px] font-bold uppercase tracking-[0.14em] text-ink-faint"
          >
            {t("menu.displaySettings")}
          </h3>
          <div className="mt-2 grid gap-4 rounded border border-line bg-card p-4">
            <label className="grid gap-1.5 text-xs font-medium text-ink-muted">
              {t("menu.theme")}
              <select
                aria-label={t("menu.theme")}
                value={theme}
                onChange={(event) => setTheme(event.target.value as Theme)}
                className={`${autoControl} h-10 w-full text-sm`}
              >
                {themes.map((candidate) => (
                  <option key={candidate} value={candidate}>
                    {t(themeLabels[candidate])}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-1.5 text-xs font-medium text-ink-muted">
              {t("menu.language")}
              <select
                aria-label={t("menu.language")}
                value={locale}
                onChange={(event) => setLocale(event.target.value as Locale)}
                className={`${autoControl} h-10 w-full text-sm`}
              >
                {locales.map((candidate) => (
                  <option key={candidate} value={candidate}>
                    {t(localeLabels[candidate])}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </section>
      </div>
    </section>
  );
}

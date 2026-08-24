import { InspectorToggle, type InspectorContent } from "../ui/Inspector";
import { Icon } from "../ui/icons";
import { BrandMark } from "../ui/BrandMark";
import { autoControl } from "../ui/form";
import { useLanguage } from "../i18n/context";
import { locales, type Locale } from "../i18n/locale";
import { themes, type Theme } from "../theme/theme";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";

export function AppHeader({
  route,
  version,
  state,
  navigationOpen,
  desktopNavigationVisible,
  navigationId,
  onToggleNavigation,
  onToggleDesktopNavigation,
  inspector,
  inspectorOpen,
  onToggleInspector,
  sectionLabels,
  themeLabels,
  localeLabels,
  theme,
  onThemeChange,
}: {
  route: { kind: string; section?: Section };
  version: string;
  state: string;
  navigationOpen: boolean;
  desktopNavigationVisible: boolean;
  navigationId: string;
  onToggleNavigation: () => void;
  onToggleDesktopNavigation: () => void;
  inspector: InspectorContent | null;
  inspectorOpen: boolean;
  onToggleInspector: () => void;
  sectionLabels: Record<Section, MessageKey>;
  themeLabels: Record<Theme, MessageKey>;
  localeLabels: Record<Locale, MessageKey>;
  theme: Theme;
  onThemeChange: (theme: Theme) => void;
}) {
  const { t, locale, setLocale } = useLanguage();
  return (
    <header className="relative z-20 flex shrink-0 flex-col gap-2 border-b border-line bg-toolbar px-3 py-2 md:min-h-12 md:flex-row md:items-center md:gap-3 md:px-5">
      <div className="flex w-full min-w-0 items-center gap-2 md:contents">
        <button
          type="button"
          aria-label={t("shell.navigationToggle")}
          aria-expanded={navigationOpen}
          aria-controls={navigationId}
          onClick={() => onToggleNavigation()}
          className="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-control-line bg-card text-ink shadow-sm md:hidden"
        >
          <Icon name="menu" className="h-4 w-4" />
        </button>

        <button
          type="button"
          aria-label={t(desktopNavigationVisible ? "shell.navigationHide" : "shell.navigationShow")}
          aria-expanded={desktopNavigationVisible}
          aria-controls={navigationId}
          title={t(desktopNavigationVisible ? "shell.navigationHide" : "shell.navigationShow")}
          onClick={() => onToggleDesktopNavigation()}
          className="hidden h-8 w-8 shrink-0 place-items-center rounded-lg border border-control-line bg-card text-ink shadow-sm md:grid"
        >
          <Icon name="menu" className="h-4 w-4" />
        </button>

        <div className="hidden shrink-0 items-center gap-2 md:flex">
          <BrandMark className="h-7 w-7 drop-shadow-sm" />
          <h1 className="whitespace-nowrap text-sm font-bold tracking-tight">{t("shell.title")}</h1>
        </div>

        <span aria-hidden="true" className="hidden h-5 w-px bg-line md:block" />
        <p className="min-w-0 flex-1 truncate text-base font-semibold md:text-sm">
          {route.kind === "section" && route.section !== undefined
            ? t(sectionLabels[route.section])
            : t("shell.pageNotFound")}
        </p>

        <p role="status" className="sr-only">
          {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
        </p>

        <div
          aria-hidden="true"
          data-session-status-badge
          className="hidden min-w-0 shrink-0 items-center gap-2 rounded-full border border-line bg-card px-2 py-1 text-xs text-ink-muted shadow-sm md:flex"
        >
          <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-live shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-live)_14%,transparent)]" />
          <span className="hidden lg:block lg:max-w-52 lg:truncate">
            {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
          </span>
        </div>

        {inspector === null ? null : (
          <span className="shrink-0 [&>button]:h-10 [&>button]:min-w-10 [&>button]:justify-center md:contents md:[&>button]:h-auto md:[&>button]:min-w-0">
            <InspectorToggle
              label={inspector.label}
              open={inspectorOpen}
              attention={inspector.attention}
              onToggle={() => onToggleInspector()}
            />
          </span>
        )}
      </div>

      <div className="flex w-full shrink-0 items-center gap-1 rounded-lg border border-control-line bg-control p-0.5 shadow-sm md:w-auto">
        <label htmlFor="appearance" className="sr-only">{t("shell.theme")}</label>
        <select
          id="appearance"
          value={theme}
          onChange={(event) => onThemeChange(event.target.value as Theme)}
          className={`${autoControl} h-10 min-w-0 basis-0 grow border-0 bg-transparent px-2 py-1 text-sm shadow-none md:h-auto md:w-auto md:basis-auto md:grow-0 md:px-1.5 md:text-xs`}
        >
          {themes.map((candidate) => (
            <option key={candidate} value={candidate}>
              {t(themeLabels[candidate])}
            </option>
          ))}
        </select>
        <span aria-hidden="true" className="h-4 w-px bg-line" />
        <label htmlFor="language" className="sr-only">{t("shell.language")}</label>
        <select
          id="language"
          value={locale}
          onChange={(event) => setLocale(event.target.value as Locale)}
          className={`${autoControl} h-10 min-w-0 basis-0 grow border-0 bg-transparent px-2 py-1 text-sm shadow-none md:h-auto md:w-auto md:basis-auto md:grow-0 md:px-1.5 md:text-xs`}
        >
          {locales.map((candidate) => (
            <option key={candidate} value={candidate}>
              {t(localeLabels[candidate])}
            </option>
          ))}
        </select>
      </div>
    </header>
  );
}

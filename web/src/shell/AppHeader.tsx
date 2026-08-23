import { InspectorToggle, type InspectorContent } from "../ui/Inspector";
import { Icon } from "../ui/icons";
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
  navigationId,
  onToggleNavigation,
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
  navigationId: string;
  onToggleNavigation: () => void;
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
  <header className="relative z-20 flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2.5 md:gap-3 md:px-6">
    <button
      type="button"
      aria-label={t("shell.navigationToggle")}
      aria-expanded={navigationOpen}
      aria-controls={navigationId}
      onClick={() => onToggleNavigation()}
      className="shrink-0 rounded-md border border-control-line bg-card p-2 md:hidden"
    >
      <Icon name="menu" className="h-4 w-4" />
    </button>

    <h1 className="hidden shrink-0 whitespace-nowrap text-xs font-medium text-ink-muted md:block">{t("shell.title")}</h1>
    <span aria-hidden="true" className="hidden text-xs text-ink-faint md:inline">/</span>
    <p className="shrink-0 whitespace-nowrap text-sm font-semibold">
      {route.kind === "section" && route.section !== undefined
        ? t(sectionLabels[route.section])
        : t("shell.pageNotFound")}
    </p>

    <p role="status" className="flex min-w-0 items-center gap-1.5 truncate text-xs text-ink-muted">
      <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-live" />
      <span className="sr-only sm:not-sr-only sm:truncate">
        {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
      </span>
    </p>
    {inspector === null ? (
      <span className="ml-auto" />
    ) : (
      <span className="ml-auto">
        <InspectorToggle
          label={inspector.label}
          open={inspectorOpen}
          attention={inspector.attention}
          onToggle={() => onToggleInspector()}
        />
      </span>
    )}
    <label htmlFor="appearance" className="hidden shrink-0 whitespace-nowrap text-sm text-ink-muted md:inline">
      {t("shell.theme")}
    </label>

    <select
      id="appearance"
      value={theme}
      onChange={(event) => onThemeChange(event.target.value as Theme)}
      className={`${autoControl} min-w-0 max-w-24 md:max-w-none`}
    >
      {themes.map((candidate) => (
        <option key={candidate} value={candidate}>
          {t(themeLabels[candidate])}
        </option>
      ))}
    </select>
    <label htmlFor="language" className="hidden shrink-0 whitespace-nowrap text-sm text-ink-muted md:inline">
      {t("shell.language")}
    </label>
    <select
      id="language"
      value={locale}
      onChange={(event) => setLocale(event.target.value as Locale)}
      className={`${autoControl} min-w-0 max-w-24 md:max-w-none`}
    >
      {locales.map((candidate) => (
        <option key={candidate} value={candidate}>
          {t(localeLabels[candidate])}
        </option>
      ))}
    </select>
  </header>
  );
}

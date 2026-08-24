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
    <header className="relative z-20 flex min-h-12 shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2 md:gap-3 md:px-5">
      <button
        type="button"
        aria-label={t("shell.navigationToggle")}
        aria-expanded={navigationOpen}
        aria-controls={navigationId}
        onClick={() => onToggleNavigation()}
        className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-control-line bg-card text-ink shadow-sm md:hidden"
      >
        <Icon name="menu" className="h-4 w-4" />
      </button>

      <div className="flex shrink-0 items-center gap-2">
        <span
          aria-hidden="true"
          className="hidden h-7 w-7 place-items-center rounded-lg bg-accent font-mono text-[10px] font-bold tracking-tighter text-accent-ink shadow-sm sm:grid"
        >
          &gt;_
        </span>
        <h1 className="hidden whitespace-nowrap text-sm font-bold tracking-tight md:block">{t("shell.title")}</h1>
      </div>

      <span aria-hidden="true" className="hidden h-5 w-px bg-line md:block" />
      <p className="min-w-0 truncate text-sm font-semibold">
        {route.kind === "section" && route.section !== undefined
          ? t(sectionLabels[route.section])
          : t("shell.pageNotFound")}
      </p>

      <span className="min-w-0 flex-1" />
      <p
        role="status"
        className="flex min-w-0 shrink-0 items-center gap-2 rounded-full border border-line bg-card px-2 py-1 text-xs text-ink-muted shadow-sm"
      >
        <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-live shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-live)_14%,transparent)]" />
        <span className="sr-only lg:not-sr-only lg:max-w-52 lg:truncate">
          {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
        </span>
      </p>

      {inspector === null ? null : (
        <InspectorToggle
          label={inspector.label}
          open={inspectorOpen}
          attention={inspector.attention}
          onToggle={() => onToggleInspector()}
        />
      )}

      <div className="flex shrink-0 items-center gap-1 rounded-lg border border-control-line bg-control p-0.5 shadow-sm">
        <label htmlFor="appearance" className="sr-only">{t("shell.theme")}</label>
        <select
          id="appearance"
          value={theme}
          onChange={(event) => onThemeChange(event.target.value as Theme)}
          className={`${autoControl} w-[4.75rem] border-0 bg-transparent px-1.5 py-1 text-xs shadow-none sm:w-auto`}
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
          className={`${autoControl} w-16 border-0 bg-transparent px-1.5 py-1 text-xs shadow-none sm:w-auto`}
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

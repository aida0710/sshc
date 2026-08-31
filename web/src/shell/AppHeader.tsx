import { InspectorToggle, type InspectorContent } from "../ui/Inspector";
import { Icon } from "../ui/icons";
import { BrandMark } from "../ui/BrandMark";
import { autoControl } from "../ui/form";
import { useLanguage } from "../i18n/context";
import { locales, type Locale } from "../i18n/locale";
import { themes, type Theme } from "../theme/theme";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import { useRef, useState, useSyncExternalStore, type RefObject } from "react";
import { sftpTransferManager } from "../sftp/transferManager";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";

export function AppHeader({
  route,
  version,
  state,
  navigationOpen,
  desktopNavigationVisible,
  navigationId,
  navigationToggleRef,
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
  onOpenCommandPalette,
  onOpenTransfers,
}: {
  route: { kind: string; section?: Section };
  version: string;
  state: string;
  navigationOpen: boolean;
  desktopNavigationVisible: boolean;
  navigationId: string;
  navigationToggleRef?: RefObject<HTMLButtonElement | null>;
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
  onOpenCommandPalette: (trigger: HTMLElement) => void;
  onOpenTransfers: () => void;
}) {
  const { t, locale, setLocale } = useLanguage();
  const preferenceMenu = useRef<HTMLDetailsElement>(null);
  const [preferenceMenuOpen, setPreferenceMenuOpen] = useState(false);
  const transfers = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const activeTransfers = transfers.filter((job) => ["queued", "running", "paused", "reattach", "needs_overwrite"].includes(job.status)).length;
  useDismissibleLayer({
    open: preferenceMenuOpen,
    containerRefs: [preferenceMenu],
    onDismiss: () => setPreferenceMenuOpen(false),
  });
  return (
    <header data-app-header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-2 border-b border-line bg-toolbar px-2 md:gap-3 md:px-3">
      <div className="flex min-w-0 flex-1 items-center gap-2 md:contents">
        <button
          ref={navigationToggleRef}
          type="button"
          aria-label={t("shell.navigationToggle")}
          aria-expanded={navigationOpen}
          aria-controls={navigationId}
          onClick={() => onToggleNavigation()}
          className="grid h-10 w-10 shrink-0 place-items-center rounded text-ink-muted hover:bg-select-fill hover:text-ink md:hidden"
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
          className="hidden h-8 w-8 shrink-0 place-items-center rounded text-ink-muted hover:bg-select-fill hover:text-ink md:grid"
        >
          <Icon name="menu" className="h-4 w-4" />
        </button>

        <div className="hidden shrink-0 items-center gap-2 md:flex">
          <BrandMark className="h-6 w-6" />
          <h1 className="whitespace-nowrap font-mono text-sm font-semibold tracking-tight">{t("shell.title")}</h1>
        </div>

        <span aria-hidden="true" className="hidden h-5 w-px bg-line md:block" />
        <p className="min-w-0 flex-1 truncate text-sm font-semibold">
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
          className="hidden min-w-0 shrink-0 items-center gap-2 px-1 text-xs text-ink-muted md:flex"
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

      <div className="hidden shrink-0 items-center gap-1 border-l border-line pl-2 md:flex">
        <button
          type="button"
          aria-label={t("palette.open")}
          onClick={(event) => onOpenCommandPalette(event.currentTarget)}
          className="mr-1 flex h-8 min-w-44 items-center gap-2 rounded border border-control-line bg-control px-2.5 text-xs text-ink-muted hover:border-accent hover:text-ink"
        >
          <Icon name="search" className="h-3.5 w-3.5" />
          <span className="flex-1 text-left">{t("palette.open")}</span>
          <kbd className="font-mono text-[10px] text-ink-faint">Ctrl K</kbd>
        </button>
        {activeTransfers > 0 ? (
          <button
            type="button"
            onClick={onOpenTransfers}
            className="mr-1 flex h-8 items-center gap-1.5 border-l border-line px-2 text-xs text-ink-muted hover:text-ink"
          >
            <span aria-hidden="true" className="size-1.5 rounded-full bg-notice-ink" />
            {t("sftp.activeTransfers", { count: activeTransfers })}
          </button>
        ) : null}
        <label htmlFor="appearance" className="sr-only">
          {t("shell.theme")}
        </label>
        <select
          id="appearance"
          value={theme}
          onChange={(event) => onThemeChange(event.target.value as Theme)}
          className={`${autoControl} h-8 max-w-32 border-0 bg-transparent px-1.5 text-xs`}
        >
          {themes.map((candidate) => (
            <option key={candidate} value={candidate}>
              {t(themeLabels[candidate])}
            </option>
          ))}
        </select>
        <span aria-hidden="true" className="h-4 w-px bg-line" />
        <label htmlFor="language" className="sr-only">
          {t("shell.language")}
        </label>
        <select
          id="language"
          value={locale}
          onChange={(event) => setLocale(event.target.value as Locale)}
          className={`${autoControl} h-8 max-w-24 border-0 bg-transparent px-1.5 text-xs`}
        >
          {locales.map((candidate) => (
            <option key={candidate} value={candidate}>
              {t(localeLabels[candidate])}
            </option>
          ))}
        </select>
      </div>

      <details
        ref={preferenceMenu}
        open={preferenceMenuOpen}
        onToggle={(event) => setPreferenceMenuOpen(event.currentTarget.open)}
        className="group relative shrink-0 md:hidden"
      >
        <summary
          aria-label={t("shell.preferenceMenu")}
          className="grid h-10 w-10 cursor-pointer list-none place-items-center rounded text-ink-muted marker:hidden hover:bg-select-fill hover:text-ink"
        >
          <Icon name="moreHorizontal" className="h-4 w-4" />
        </summary>
        <div className="absolute right-0 top-[calc(100%+0.4rem)] z-50 grid w-64 gap-3 rounded border border-control-line bg-card p-3 shadow-2xl">
          <div className="grid gap-1 text-xs font-medium text-ink-muted">
            <span aria-hidden="true">{t("shell.theme")}</span>
            <select
              id="mobile-appearance"
              aria-label={t("shell.themeMenu")}
              value={theme}
              onChange={(event) => onThemeChange(event.target.value as Theme)}
              className={`${autoControl} h-10 w-full text-sm`}
            >
              {themes.map((candidate) => (
                <option key={candidate} value={candidate}>
                  {t(themeLabels[candidate])}
                </option>
              ))}
            </select>
          </div>
          <div className="grid gap-1 text-xs font-medium text-ink-muted">
            <span aria-hidden="true">{t("shell.language")}</span>
            <select
              id="mobile-language"
              aria-label={t("shell.languageMenu")}
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
          </div>
        </div>
      </details>
    </header>
  );
}

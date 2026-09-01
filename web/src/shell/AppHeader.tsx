import { InspectorToggle, type InspectorContent } from "../ui/Inspector";
import { Icon } from "../ui/icons";
import { useLanguage } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import { type RefObject } from "react";

export function AppHeader({
  route,
  navigationOpen,
  navigationId,
  navigationToggleRef,
  onToggleNavigation,
  inspector,
  inspectorOpen,
  inspectorToggleRef,
  onToggleInspector,
  sectionLabels,
}: {
  route: { kind: string; section?: Section };
  navigationOpen: boolean;
  navigationId: string;
  navigationToggleRef?: RefObject<HTMLButtonElement | null>;
  onToggleNavigation: () => void;
  inspector: InspectorContent | null;
  inspectorOpen: boolean;
  inspectorToggleRef?: RefObject<HTMLButtonElement | null>;
  onToggleInspector: () => void;
  sectionLabels: Record<Section, MessageKey>;
}) {
  const { t } = useLanguage();
  return (
    <header data-app-header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-2 border-b border-line bg-toolbar px-2 md:hidden">
      <button
        ref={navigationToggleRef}
        type="button"
        aria-label={t("shell.navigationToggle")}
        aria-expanded={navigationOpen}
        aria-controls={navigationId}
        onClick={onToggleNavigation}
        className="grid h-10 w-10 shrink-0 place-items-center rounded text-ink-muted hover:bg-select-fill hover:text-ink"
      >
        <Icon name="menu" className="h-4 w-4" />
      </button>

      <p className="min-w-0 flex-1 truncate text-sm font-semibold">
        {route.kind === "section" && route.section !== undefined
          ? t(sectionLabels[route.section])
          : t("shell.pageNotFound")}
      </p>

      {inspector === null ? null : (
        <span className="shrink-0 [&>button]:h-10 [&>button]:min-w-10 [&>button]:justify-center">
          <InspectorToggle
            label={inspector.label}
            open={inspectorOpen}
            attention={inspector.attention}
            {...(inspectorToggleRef === undefined ? {} : { buttonRef: inspectorToggleRef })}
            onToggle={onToggleInspector}
          />
        </span>
      )}

    </header>
  );
}

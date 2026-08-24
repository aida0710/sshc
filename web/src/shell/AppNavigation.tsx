import type { MouseEvent, ReactNode } from "react";
import { ConsoleList } from "../terminal/ConsoleList";
import { UpdateBadge } from "./UpdateBadge";
import { Icon, type IconName } from "../ui/icons";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSession } from "../api/integrations";

export function AppNavigation({
  navigationId,
  navigationOpen,
  navGroups,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
  currentFace,
  onFaceChange,
  consoles,
  orderedConsoles,
  activeConsole,
  onShowConsole,
  onDuplicateConsole,
  onReorderConsoles,
  onOpenShell,
}: {
  navigationId: string;
  navigationOpen: boolean;
  navGroups: { label: MessageKey; sections: Section[] }[];
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
  currentFace: NavFace;
  onFaceChange: (face: NavFace) => void;
  consoles: TerminalSessionsState;
  orderedConsoles: TerminalSession[];
  activeConsole: string | null;
  onShowConsole: (id: string) => void;
  onDuplicateConsole: (id: string) => void;
  onReorderConsoles: (order: string[]) => void;
  onOpenShell: () => void;
}) {
  const t = useTranslate();
  const navigationLink = (name: Section) => (
    <NavigationLink
      name={name}
      section={section}
      sectionIcons={sectionIcons}
      sectionLabels={sectionLabels}
      onNavigate={onNavigate}
    />
  );
  return (
    <nav
      id={navigationId}
      aria-label={t("shell.primaryNavigation")}
      className={`fixed inset-y-0 left-0 z-30 flex min-h-0 w-72 flex-col overflow-hidden border-r border-line bg-sidebar p-3 shadow-2xl transition-transform md:static md:z-auto md:w-auto md:translate-x-0 md:shadow-none ${
        navigationOpen ? "translate-x-0" : "-translate-x-full"
      }`}
    >
      <div className="mb-3 flex shrink-0 items-center gap-2 border-b border-line px-1 pb-3 md:hidden">
        <span
          aria-hidden="true"
          className="grid h-8 w-8 place-items-center rounded-lg bg-accent font-mono text-[11px] font-bold tracking-tighter text-accent-ink"
        >
          &gt;_
        </span>
        <span className="text-sm font-bold tracking-tight">{t("shell.title")}</span>
      </div>

      <div className="shrink-0">
        {navGroups.slice(0, 1).map((group) => (
          <NavigationGroup key={group.label} label={t(group.label)}>
            {group.sections.map((name) => (
              <li key={name}>{navigationLink(name)}</li>
            ))}
          </NavigationGroup>
        ))}
      </div>

      <div
        role="tablist"
        aria-label={t("shell.navFaces")}
        className="my-3 grid shrink-0 grid-cols-2 gap-1 rounded-xl border border-control-line bg-control p-1 md:my-2"
      >
        {(["settings", "terminal"] as NavFace[]).map((face) => (
          <button
            key={face}
            type="button"
            role="tab"
            aria-selected={face === currentFace}
            onClick={() => onFaceChange(face)}
            className={`flex items-center justify-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors ${
              face === currentFace
                ? "bg-card text-ink shadow-sm"
                : "text-ink-muted hover:bg-card/60 hover:text-ink"
            }`}
          >
            <Icon name={face === "settings" ? "settings" : "terminal"} className="h-3.5 w-3.5" />
            {t(face === "settings" ? "shell.navFaceSettings" : "shell.navFaceTerminal")}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto pr-0.5">
        {currentFace === "terminal" ? (
          <ConsoleList
            sessions={orderedConsoles}
            selected={activeConsole}
            maxSessions={consoles.maxSessions}
            busy={consoles.busy}
            problem={consoles.problem}
            onSelect={onShowConsole}
            onClose={(id) => void consoles.close(id)}
            onRename={(id, title) => consoles.rename(id, title)}
            onDuplicate={onDuplicateConsole}
            onReorder={onReorderConsoles}
            onOpenShell={onOpenShell}
          />
        ) : (
          navGroups.slice(1).map((group) => (
            <NavigationGroup key={group.label} label={t(group.label)}>
              {group.sections.map((name) => (
                <li key={name}>{navigationLink(name)}</li>
              ))}
            </NavigationGroup>
          ))
        )}
      </div>

      <div className="shrink-0 border-t border-line pt-2">
        <UpdateBadge />
      </div>
    </nav>
  );
}

export type NavFace = "settings" | "terminal";

function NavigationLink({
  name,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
}: {
  name: Section;
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
}) {
  const t = useTranslate();
  const active = section === name;
  return (
    <a
      href={sectionPath(name)}
      aria-current={active ? "page" : undefined}
      onClick={(event) => {
        onNavigate(event, name);
      }}
      className={`group relative my-0.5 flex w-full items-center gap-2.5 overflow-hidden rounded-lg px-2 py-2.5 text-left text-sm transition-colors md:py-1 ${
        active
          ? "bg-card font-semibold text-ink shadow-sm"
          : "font-medium text-ink-muted hover:bg-select-fill hover:text-ink"
      }`}
    >
      {active ? <span aria-hidden="true" className="absolute inset-y-2 left-0 w-0.5 rounded-r-full bg-accent" /> : null}
      <span
        aria-hidden="true"
        className={`grid h-7 w-7 shrink-0 place-items-center rounded-md transition-colors md:h-6 md:w-6 ${
          active ? "bg-accent text-accent-ink" : "bg-control text-ink-muted group-hover:text-ink"
        }`}
      >
        <Icon name={sectionIcons[name]} className="h-4 w-4" />
      </span>
      <span className="truncate">{t(sectionLabels[name])}</span>
    </a>
  );
}

function NavigationGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="mb-3 md:mb-2">
      <span
        aria-hidden="true"
        className="block px-2 pb-1 pt-1 text-[10px] font-bold uppercase tracking-[0.14em] text-ink-faint md:pb-0.5"
      >
        {label}
      </span>
      <ul aria-label={label}>{children}</ul>
    </div>
  );
}

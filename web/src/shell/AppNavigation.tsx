import {
  useEffect,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import { ConsoleList } from "../terminal/ConsoleList";
import { UpdateBadge } from "./UpdateBadge";
import { Icon, type IconName } from "../ui/icons";
import { BrandMark } from "../ui/BrandMark";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { TerminalSession } from "../api/integrations";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import {
  clampNavigationWidth,
  maximumNavigationWidth,
  minimumNavigationWidth,
} from "./navigationLayout";

export function AppNavigation({
  navigationId,
  navigationOpen,
  desktopVisible,
  desktopWidth,
  onDesktopWidthChange,
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
  liveWorkspace,
  onShowConsole,
  onDuplicateConsole,
  onReorderConsoles,
  onOpenShell,
  onOpenCommandPalette,
}: {
  navigationId: string;
  navigationOpen: boolean;
  desktopVisible: boolean;
  desktopWidth: number;
  onDesktopWidthChange: (width: number) => void;
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
  liveWorkspace: LiveWorkspaceSummary | null;
  onShowConsole: (id: string) => void;
  onDuplicateConsole: (id: string) => void;
  onReorderConsoles: (order: string[]) => void;
  onOpenShell: () => void;
  onOpenCommandPalette: () => void;
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
      className={`fixed inset-y-0 left-0 z-30 flex min-h-0 w-72 max-w-[calc(100vw-2rem)] flex-col overflow-hidden border-r border-line bg-sidebar p-2 transition-transform motion-reduce:transition-none md:relative md:inset-auto md:z-auto md:w-auto md:max-w-none md:translate-x-0 md:shadow-none ${
        navigationOpen ? "translate-x-0 shadow-2xl" : "-translate-x-full shadow-none"
      } ${desktopVisible ? "md:flex" : "md:hidden"}`}
    >
      <div className="mb-1 flex h-10 shrink-0 items-center gap-2 border-b border-line px-1 md:hidden">
        <BrandMark className="h-7 w-7" />
        <span className="font-mono text-sm font-semibold tracking-tight">{t("shell.title")}</span>
      </div>

      <button
        type="button"
        onClick={onOpenCommandPalette}
        className="mb-1 flex h-10 shrink-0 items-center gap-2 rounded border border-control-line bg-control px-2.5 text-sm text-ink-muted hover:text-ink md:hidden"
      >
        <Icon name="search" className="h-4 w-4" />
        <span className="flex-1 text-left">{t("palette.open")}</span>
      </button>

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
        className="my-1.5 grid shrink-0 grid-cols-2 border-y border-line md:my-2"
      >
        {(["settings", "terminal"] as NavFace[]).map((face) => (
          <button
            key={face}
            type="button"
            role="tab"
            aria-selected={face === currentFace}
            onClick={() => onFaceChange(face)}
            className={`flex items-center justify-center gap-1.5 border-b-2 px-2 py-2 text-xs font-medium transition-colors ${
              face === currentFace
                ? "border-accent text-ink"
                : "border-transparent text-ink-muted hover:bg-select-fill hover:text-ink"
            }`}
          >
            <Icon name={face === "settings" ? "settings" : "terminal"} className="h-3.5 w-3.5" />
            {t(face === "settings" ? "shell.navFaceSettings" : "shell.navFaceTerminal")}
          </button>
        ))}
      </div>

      <div data-navigation-scroll className="min-h-0 flex-1 overflow-y-auto pr-0.5">
        {currentFace === "terminal" ? (
          <ConsoleList
            sessions={orderedConsoles}
            selected={activeConsole}
            workspace={liveWorkspace}
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

      <div className="shrink-0 pt-1 md:pt-2">
        <UpdateBadge />
      </div>

      <NavigationResizeHandle width={desktopWidth} onWidthChange={onDesktopWidthChange} />
    </nav>
  );
}

export type NavFace = "settings" | "terminal";

export function NavigationResizeHandle({
  width,
  onWidthChange,
}: {
  width: number;
  onWidthChange: (width: number) => void;
}) {
  const t = useTranslate();
  const drag = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(null);
  const queuedWidth = useRef(width);
  const animationFrame = useRef<number | null>(null);
  const previousUserSelect = useRef("");

  useEffect(() => {
    queuedWidth.current = width;
  }, [width]);

  useEffect(() => () => {
    if (animationFrame.current !== null) window.cancelAnimationFrame(animationFrame.current);
    if (drag.current !== null) document.body.style.userSelect = previousUserSelect.current;
  }, []);

  function publish(nextWidth: number) {
    queuedWidth.current = clampNavigationWidth(nextWidth);
    if (animationFrame.current !== null) return;
    animationFrame.current = window.requestAnimationFrame(() => {
      animationFrame.current = null;
      onWidthChange(queuedWidth.current);
    });
  }

  function finish(pointerId: number) {
    if (drag.current?.pointerId !== pointerId) return;
    drag.current = null;
    document.body.style.userSelect = previousUserSelect.current;
    if (animationFrame.current !== null) {
      window.cancelAnimationFrame(animationFrame.current);
      animationFrame.current = null;
      onWidthChange(queuedWidth.current);
    }
  }

  function start(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || drag.current !== null) return;
    event.preventDefault();
    drag.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width };
    queuedWidth.current = width;
    previousUserSelect.current = document.body.style.userSelect;
    document.body.style.userSelect = "none";
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function move(event: ReactPointerEvent<HTMLDivElement>) {
    const active = drag.current;
    if (active === null || active.pointerId !== event.pointerId) return;
    publish(active.startWidth + event.clientX - active.startX);
  }

  function useKeyboard(event: ReactKeyboardEvent<HTMLDivElement>) {
    let nextWidth: number | null = null;
    const step = event.shiftKey ? 32 : 8;
    if (event.key === "ArrowLeft") nextWidth = width - step;
    if (event.key === "ArrowRight") nextWidth = width + step;
    if (event.key === "Home") nextWidth = minimumNavigationWidth;
    if (event.key === "End") nextWidth = maximumNavigationWidth;
    if (nextWidth === null) return;
    event.preventDefault();
    onWidthChange(clampNavigationWidth(nextWidth));
  }

  return (
    <div
      role="separator"
      aria-label={t("shell.navigationResize")}
      aria-orientation="vertical"
      aria-valuemin={minimumNavigationWidth}
      aria-valuemax={maximumNavigationWidth}
      aria-valuenow={width}
      tabIndex={0}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={(event) => finish(event.pointerId)}
      onPointerCancel={(event) => finish(event.pointerId)}
      onLostPointerCapture={(event) => finish(event.pointerId)}
      onKeyDown={useKeyboard}
      className="group absolute inset-y-0 right-0 hidden w-2 cursor-col-resize touch-none items-center justify-center outline-none md:flex"
    >
      <span
        aria-hidden="true"
        className="h-full w-px bg-transparent transition-colors group-hover:bg-accent group-focus-visible:w-0.5 group-focus-visible:bg-accent"
      />
    </div>
  );
}

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
      className={`group relative flex min-h-10 w-full items-center gap-2 overflow-hidden rounded-sm px-2 py-1 text-left text-sm transition-colors md:my-px md:min-h-8 md:gap-2.5 md:py-0.5 ${
        active
          ? "bg-select-fill font-semibold text-ink"
          : "font-medium text-ink-muted hover:bg-select-fill hover:text-ink"
      }`}
    >
      {active ? <span aria-hidden="true" className="absolute inset-y-1 left-0 w-0.5 bg-accent" /> : null}
      <span
        aria-hidden="true"
        className={`grid h-6 w-6 shrink-0 place-items-center transition-colors md:h-5 md:w-5 ${
          active ? "text-ink" : "text-ink-muted group-hover:text-ink"
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
    <div className="mb-1 md:mb-1.5">
      <span
        aria-hidden="true"
        className="block px-2 pb-0.5 pt-0.5 text-[10px] font-bold uppercase tracking-[0.14em] text-ink-faint"
      >
        {label}
      </span>
      <ul aria-label={label}>{children}</ul>
    </div>
  );
}

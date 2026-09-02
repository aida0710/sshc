import {
  useEffect,
  useRef,
  useSyncExternalStore,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { ConsoleList } from "../terminal/ConsoleList";
import { UpdateBadge } from "./UpdateBadge";
import { Icon, type IconName } from "../ui/icons";
import { BrandMark } from "../ui/BrandMark";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { LocalShellProfile, TerminalSession } from "../api/integrations";
import type { LiveWorkspaceSummary } from "../features/workspaces/live";
import type { AgentUnreadBySession } from "../terminal/agentNotifications";
import {
  clampNavigationWidth,
  maximumNavigationWidth,
  minimumNavigationWidth,
} from "./navigationLayout";
import { sftpTransferManager } from "../sftp/transferManager";

export function AppNavigation({
  navigationRef,
  navigationId,
  version,
  state,
  navigationOpen,
  desktopWidth,
  onDesktopWidthChange,
  startSections,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
  consoles,
  orderedConsoles,
  activeConsole,
  liveWorkspace,
  onRenameWorkspace,
  unreadBySession,
  onShowConsole,
  onDuplicateConsole,
  onReorderConsoles,
  localShellProfiles = [],
  onOpenShell,
  onOpenCommandPalette,
}: {
  navigationRef?: RefObject<HTMLElement | null>;
  navigationId: string;
  version: string;
  state: string;
  navigationOpen: boolean;
  desktopWidth: number;
  onDesktopWidthChange: (width: number) => void;
  startSections: Section[];
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
  consoles: TerminalSessionsState;
  orderedConsoles: TerminalSession[];
  activeConsole: string | null;
  liveWorkspace: LiveWorkspaceSummary | null;
  onRenameWorkspace: (name: string) => void;
  unreadBySession: AgentUnreadBySession;
  onShowConsole: (id: string) => void;
  onDuplicateConsole: (id: string) => void;
  onReorderConsoles: (order: string[]) => void;
  localShellProfiles?: LocalShellProfile[];
  onOpenShell: (profileId?: string) => void;
  onOpenCommandPalette: () => void;
}) {
  const t = useTranslate();
  const transfers = useSyncExternalStore(sftpTransferManager.subscribe, sftpTransferManager.getSnapshot);
  const activeTransfers = transfers.filter((job) =>
    ["queued", "running", "paused", "reattach", "needs_overwrite"].includes(job.status),
  ).length;
  const navigationLink = (name: Section) => (
    <NavigationLink
      name={name}
      section={section}
      sectionIcons={sectionIcons}
      sectionLabels={sectionLabels}
      onNavigate={onNavigate}
      attention={name === "Files" && activeTransfers > 0}
      attentionLabel={name === "Files" && activeTransfers > 0 ? t("sftp.activeTransfers", { count: activeTransfers }) : undefined}
    />
  );
  return (
    <nav
      ref={navigationRef}
      id={navigationId}
      aria-label={t("shell.primaryNavigation")}
      className={`fixed inset-y-0 left-0 z-30 flex min-h-0 w-72 max-w-[calc(100vw-2rem)] flex-col overflow-hidden border-r border-line bg-sidebar p-2 transition-transform motion-reduce:transition-none md:relative md:inset-auto md:z-auto md:flex md:w-auto md:max-w-none md:translate-x-0 md:shadow-none ${
        navigationOpen ? "translate-x-0 shadow-2xl" : "-translate-x-full shadow-none"
      }`}
    >
      <div data-navigation-heading className="mb-1 flex h-10 shrink-0 items-center gap-2 border-b border-line px-1">
        <BrandMark className="h-6 w-6" />
        <h1 className="font-mono text-sm font-semibold tracking-tight">{t("shell.title")}</h1>
        <span aria-hidden="true" className="h-4 w-px bg-line" />
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">
          {section === null ? t("shell.pageNotFound") : t(sectionLabels[section])}
        </span>
      </div>

      <button
        type="button"
        onClick={onOpenCommandPalette}
        className="mb-1 flex h-10 shrink-0 items-center gap-2 rounded border border-control-line bg-control px-2.5 text-sm text-ink-muted hover:border-accent hover:text-ink md:h-8 md:text-xs"
      >
        <Icon name="search" className="h-4 w-4" />
        <span className="flex-1 text-left">{t("palette.open")}</span>
        <kbd className="hidden font-mono text-[10px] text-ink-faint md:block">Ctrl K</kbd>
      </button>

      <div className="shrink-0">
        <NavigationGroup label={t("shell.navStart")}>
          {startSections.map((name) => (
            <li key={name}>{navigationLink(name)}</li>
          ))}
        </NavigationGroup>
      </div>

      <div className="shrink-0 border-y border-line py-1">
        {navigationLink("Menu")}
      </div>

      <div data-navigation-scroll className="min-h-0 flex-1 overflow-y-auto pr-0.5 pt-2">
        <span
          aria-hidden="true"
          className="block px-2 pb-1 text-[10px] font-bold uppercase tracking-[0.14em] text-ink-faint"
        >
          {t("shell.sessions")}
        </span>
        <ConsoleList
          sessions={orderedConsoles}
          selected={activeConsole}
          workspace={liveWorkspace}
          onRenameWorkspace={onRenameWorkspace}
          unreadBySession={unreadBySession}
          maxSessions={consoles.maxSessions}
          busy={consoles.busy}
          problem={consoles.problem}
          onSelect={onShowConsole}
          onClose={(id) => void consoles.close(id)}
          onRename={(id, title) => consoles.rename(id, title)}
          onUnpinTitle={(id) => consoles.unpinTitle?.(id) ?? Promise.resolve(false)}
          onDuplicate={onDuplicateConsole}
          onReorder={onReorderConsoles}
          localShellProfiles={localShellProfiles}
          onOpenShell={onOpenShell}
        />
      </div>

      <div className="shrink-0 pt-1 md:pt-2">
        <p role="status" data-session-status className="sr-only">
          {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
        </p>
        <UpdateBadge
          current={version}
          indicator={(
            <span
              aria-hidden="true"
              data-session-status-badge
              title={state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
              className={`size-1.5 shrink-0 rounded-full shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-live)_14%,transparent)] ${
                state === "ready" ? "bg-live" : "bg-ink-faint"
              }`}
            />
          )}
        />
      </div>

      <NavigationResizeHandle width={desktopWidth} onWidthChange={onDesktopWidthChange} />
    </nav>
  );
}

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
  attention = false,
  attentionLabel,
}: {
  name: Section;
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
  attention?: boolean;
  attentionLabel?: string | undefined;
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
      <span className="min-w-0 grow truncate">{t(sectionLabels[name])}</span>
      {attention ? (
        <span
          aria-hidden="true"
          data-sftp-transfer-indicator
          title={attentionLabel}
          className="mr-1 size-1.5 shrink-0 rounded-full bg-notice-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-notice-ink)_12%,transparent)]"
        />
      ) : null}
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

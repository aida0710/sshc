import type { PaneEdge } from "../terminal/splitView/layout";
import { SplitGroupList } from "../terminal/splitView/SplitGroupList";
import type { SplitViewController } from "../terminal/splitView/useSplitView";
import { useBindings } from "../keyconfig/presets";
import {
  useSyncExternalStore,
  type MouseEvent,
  type RefObject,
} from "react";
import { WorkspaceTree } from "../terminal/WorkspaceTree";
import type { TerminalWorkspacesState } from "../terminal/useTerminalWorkspaces";
import { UpdateBadge } from "./UpdateBadge";
import { RunTargetSwitcher } from "./RunTargetSwitcher";
import type { RunTargetsState } from "../terminal/useRunTargets";
import { Icon, type IconName } from "../ui/icons";
import { BrandMark } from "../ui/BrandMark";
import { useTranslate } from "../i18n/context";
import { ColumnResizeHandle } from "../ui/ColumnResizeHandle";
import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import type { TerminalSessionsState } from "../terminal/sessions";
import type { LocalShellProfile } from "../api/settings";
import type { TerminalSession } from "../api/terminalSessions";
import type { HostEntry } from "../api/config";
import type { UnreadSessions } from "../terminal/terminalNotifications";
import type { SeenProgramStatus } from "../terminal/programStatus";
import { navigationWidth } from "./navigationLayout";
import { sftpTransferManager } from "../sftp/transferManager";

// A phone reaches the other sections and Menu from the bar at the bottom (MobileNavigation), so its
// navigation links only Home: the way back to that bar from a terminal, whose own top bar takes its place.
// The sessions get the height the other links took.
const phoneSections: Section[] = ["Home"];

export function AppNavigation({
  navigationRef,
  navigationId,
  version,
  state,
  navigationOpen,
  mobileLayout = false,
  desktopWidth,
  onDesktopWidthChange,
  startSections,
  section,
  sectionIcons,
  sectionLabels,
  onNavigate,
  terminalSessions,
  terminalWorkspaces,
  runTargets,
  orderedSessions,
  activeSessionId,
  unreadBySession,
  seenProgramStatus,
  onShowSession,
  splitView,
  onShowSplitGroup,
  canSplitSession,
  onSplitSession,
  onDuplicateSession,
  onReorderSessions,
  localShellProfiles = [],
  onOpenShell,
  aliases = [],
  hosts = [],
  onConnect,
  onOpenCommandPalette,
}: {
  navigationRef?: RefObject<HTMLElement | null>;
  navigationId: string;
  version: string;
  state: string;
  navigationOpen: boolean;
  mobileLayout?: boolean;
  desktopWidth: number;
  onDesktopWidthChange: (width: number) => void;
  startSections: Section[];
  section: Section | null;
  sectionIcons: Record<Section, IconName>;
  sectionLabels: Record<Section, MessageKey>;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
  terminalSessions: TerminalSessionsState;
  terminalWorkspaces: TerminalWorkspacesState;
  runTargets: RunTargetsState;
  orderedSessions: TerminalSession[];
  activeSessionId: string | null;
  unreadBySession: UnreadSessions;
  seenProgramStatus: SeenProgramStatus;
  onShowSession: (id: string) => void;
  splitView: SplitViewController;
  onShowSplitGroup: (number: number) => void;
  canSplitSession?: (id: string) => boolean;
  onSplitSession?: (id: string, edge: PaneEdge) => void;
  onDuplicateSession: (id: string) => void;
  onReorderSessions: (order: string[]) => void;
  localShellProfiles?: LocalShellProfile[];
  onOpenShell: (profileId: string | undefined, workspaceId: string) => void;
  aliases?: string[];
  hosts?: HostEntry[];
  onConnect?: (alias: string, workspaceId: string) => void;
  onOpenCommandPalette: () => void;
}) {
  const t = useTranslate();
  const shortcuts = useBindings();
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
      aria-hidden={mobileLayout && !navigationOpen ? true : undefined}
      inert={mobileLayout && !navigationOpen}
      data-navigation-open={navigationOpen}
      data-app-navigation
      className={`fixed inset-y-0 left-0 z-30 flex min-h-0 w-72 max-w-[calc(100vw-2rem)] flex-col overflow-hidden bg-sidebar px-2.5 pb-3 pt-3.5 transition-transform motion-reduce:transition-none md:relative md:inset-auto md:z-auto md:flex md:w-auto md:max-w-none md:m-2 md:mr-0 md:translate-x-0 md:rounded-xl md:border md:border-line md:shadow-pane ${
        navigationOpen ? "translate-x-0 shadow-overlay" : "-translate-x-full shadow-none"
      }`}
    >
      <div data-navigation-heading className="mb-1.5 shrink-0">
        <p role="status" data-session-status className="sr-only">
          {state === "ready" ? t("shell.active", { version }) : t("shell.starting")}
        </p>
        <UpdateBadge
          onNavigate={(event) => onNavigate(event, "About")}
          showsProgressDialog={section !== "About"}
          current={version}
          enabled={state === "ready" || state === "locked"}
          leading={(
            <>
              <BrandMark className="h-6 w-6" />
              <h1 className="text-sm font-semibold text-ink">{t("shell.title")}</h1>
              {/* The version sits at the right edge, as in the design. */}
              <span aria-hidden="true" className="grow" />
            </>
          )}
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

      <button
        type="button"
        onClick={onOpenCommandPalette}
        title={`${t("palette.open")} (${shortcuts.palette.join(" / ")})`}
        className="mb-2.5 flex h-10 shrink-0 items-center gap-2 rounded-lg bg-well px-2 text-[13px] text-ink-faint transition-colors hover:text-ink md:h-8"
      >
        <Icon name="search" className="size-[15px] shrink-0" />
        <span className="min-w-0 flex-1 truncate text-left">{t("palette.open")}</span>
        <kbd aria-hidden="true" className="hidden shrink-0 font-mono text-[11px] text-ink-faint md:block">{shortcuts.palette[0]}</kbd>
      </button>

      <div className="shrink-0">
        <ul aria-label={t("shell.navStart")} className="flex flex-col gap-0.5">
          {(mobileLayout ? phoneSections : startSections).map((name) => (
            <li key={name}>{navigationLink(name)}</li>
          ))}
        </ul>
      </div>

      {mobileLayout ? null : (
        <div className="mt-0.5 shrink-0">
          {navigationLink("Menu")}
        </div>
      )}

      {/* The Workspaces heading and the workspace's name under it stick to the top of this list (see
          WorkspaceTree). The scroll padding is their height, so a row brought into view by the keyboard stops
          below them rather than under them. */}
      <div data-navigation-scroll className="min-h-0 flex-1 scroll-pt-[92px] overflow-y-auto pr-0.5 md:scroll-pt-[60px]">
        <SplitGroupList groups={splitView.groups} sessions={orderedSessions}
          selected={section === "Terminal" ? splitView.activeGroupNumber : null}
          unreadBySession={unreadBySession}
          seenProgramStatus={seenProgramStatus}
          onSelect={onShowSplitGroup}
          onSelectSession={onShowSession}
          onDissolve={(number) => splitView.dispatch({ type: "dissolve-group", number })} />
        <WorkspaceTree
          workspaces={terminalWorkspaces}
          terminalSessions={terminalSessions}
          sessions={orderedSessions}
          selected={section === "Terminal" ? activeSessionId : null}
          unreadBySession={unreadBySession}
          seenProgramStatus={seenProgramStatus}
          onSelect={onShowSession}
          splitGroupBySession={splitView.groupBySession}
          {...(canSplitSession === undefined ? {} : { canSplitSession })}
          {...(onSplitSession === undefined ? {} : { onSplitSession })}
          onRemoveSessionFromSplit={splitView.removeFromSplit}
          onDuplicate={onDuplicateSession}
          onReorder={onReorderSessions}
          localShellProfiles={localShellProfiles}
          onOpenShell={onOpenShell}
          aliases={aliases}
          hosts={hosts}
          {...(onConnect === undefined ? {} : { onConnect })}
          runTarget={runTargets.selected}
        />
      </div>

      <div className="shrink-0">
        <RunTargetSwitcher targets={runTargets} aliases={aliases} hosts={hosts} />
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
  return (
    <ColumnResizeHandle
      label={t("shell.navigationResize")}
      width={width}
      minimum={navigationWidth.minimum}
      maximum={navigationWidth.maximum}
      onWidthChange={onWidthChange}
    />
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
      className={`group relative flex min-h-10 w-full items-center gap-2.5 overflow-hidden rounded-[7px] px-2 text-left text-[13px] transition-colors md:min-h-[30px] ${
        active
          ? "bg-select-fill text-ink"
          : "text-ink-muted hover:bg-hover hover:text-ink"
      }`}
    >
      <span
        aria-hidden="true"
        className={`grid size-4 shrink-0 place-items-center transition-colors ${
          active ? "text-accent" : "text-ink-muted group-hover:text-ink"
        }`}
      >
        <Icon name={sectionIcons[name]} className="size-4" />
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

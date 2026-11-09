import { useBindings } from "./keyconfig/presets";
import { usePresetLibrarySync } from "./settings/presetLibrary";
import { useSettingsSync } from "./settings/settingsStore";
import { useTerminalSettings } from "./settings/terminalSettings";
import { opensInThisPage } from "./ui/linkClick";
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { runsInAndroidApp } from "./android/native";
import { type HealthResponse } from "./api/client";
import { vaultApi, type PasswordVaultStatus } from "./api/vault";
import type { SessionState } from "./session/bootstrap";
import { addressesByAlias } from "./connections/connectionAddress";
import { blankConnectionDraft, type CreationPrerequisite } from "./connections/createConnectionDraft";
import { LockScreen } from "./secrets/LockScreen";
import { useLanguage } from "./i18n/context";
import { InspectorPane, InspectorToggle } from "./ui/Inspector";
import { PageChromeContext, type PageChrome } from "./ui/pageChrome";
import { useTheme } from "./theme/context";
import { usePolling } from "./ui/usePolling";
import { RouteSkeleton } from "./ui/RouteSkeleton";
import { sectionPath, type Section } from "./routing/sectionRoute";
import { connectionLocation } from "./routing/connectionRoute";
import { AppHeader } from "./shell/AppHeader";
import { AppNavigation } from "./shell/AppNavigation";
import { hiddenNavigationWidth, navigationShown, navigationWidth } from "./shell/navigationLayout";
import { useColumnWidth } from "./ui/useColumnWidth";
import { useStoredValue } from "./ui/useStoredValue";
import { useSectionRoute } from "./routing/useSectionRoute";
import { useTerminalSessions } from "./terminal/sessions";
import { useTerminalWorkspaces } from "./terminal/useTerminalWorkspaces";
import { SessionAddressesContext } from "./terminal/sessionDestination";
import { useRunTargetApis } from "./terminal/useRunTargetApis";
import { useRunTargetLink } from "./terminal/useRunTargetLink";
import { useRunTargets } from "./terminal/useRunTargets";
import { TransferNotifications } from "./sftp/TransferNotifications";
import { sftpTransferManager } from "./sftp/transferManager";
import { localHostAlias } from "./sftp/localHost";
import { useTransferUnloadWarning } from "./sftp/useTransferUnloadWarning";
import { ErrorDiagnosticNotice } from "./shell/ErrorDiagnosticNotice";
import { CommandPalette } from "./shell/CommandPalette";
import { MobileNavigation } from "./shell/MobileNavigation";
import { androidBackStep } from "./shell/androidBack";
import { useNavigationSwipe } from "./shell/useNavigationSwipe";
import { SectionNotices } from "./shell/SectionNotices";
import { usePaletteCommands } from "./shell/usePaletteCommands";
import { setAndroidAppearance } from "./android/native";
import { useBlockedProgramNotifications } from "./terminal/useBlockedProgramNotifications";
import { useUnseenBlockedProgramNotifications } from "./terminal/useUnseenBlockedProgramNotifications";
import { useTerminalNotifications } from "./terminal/useTerminalNotifications";
import { notificationsComeFromAndroid } from "./ui/browserNotifications";
import { useSeenProgramStatus } from "./terminal/useSeenProgramStatus";
import { useAppSession } from "./session/useAppSession";
import { useTerminalWorkspaceController } from "./terminal/useTerminalWorkspaceController";
import { mobileViewportQuery, navigationRailQuery, pinnedNavigationQuery, useMediaQuery } from "./ui/useMediaQuery";
import { useAppViewport } from "./ui/useAppViewport";
import { useFieldLetGoOnLeaving } from "./ui/useFieldLetGoOnLeaving";
import { BootstrapErrorScreen, NotFoundSection, SessionEndedScreen, VaultRecheckMark, VaultRecheckOverlay } from "./shell/AppFallbackScreens";
import { navigationId, startSections } from "./shell/sections";
import { KeptSections } from "./shell/KeptSections";
import { useKeptSections } from "./shell/useKeptSections";
import { usePreloadedSectionPages } from "./shell/usePreloadedSectionPages";
import { useFirstScreenReport } from "./android/firstScreen";
import { presentLocationChange } from "./ui/pageMotion";
import { somethingForBackToClose, useBackOwnership } from "./android/backOwnership";
import { useBackGestureMotion } from "./android/backGestureMotion";
import { sectionIcons, sectionLabels } from "./shell/sectionPresentation";
import { TerminalScreen } from "./shell/TerminalScreen";
import { useDeclaredConfig } from "./shell/useDeclaredConfig";
import { useDirectDestinationActions } from "./shell/useDirectDestinationActions";
import { useOSC52Setting } from "./shell/useOSC52Setting";
import { useSectionHandoffs } from "./shell/useSectionHandoffs";
import { useShellLayers } from "./shell/useShellLayers";
import type { AppActionContext, SessionShortcut } from "./shell/appActions";
import { useAppShortcuts, type SessionShortcutRequest } from "./shell/useAppShortcuts";
import { paneSessionIds, type PaneEdge } from "./terminal/splitView/layout";
import { SplitRequestDialog } from "./terminal/splitView/SplitRequestDialog";
import { TerminalRestorePrompt } from "./terminal/TerminalRestorePrompt";
import { useFocusBackAfterInert } from "./ui/useFocusBackAfterInert";
import { Icon } from "./ui/icons";

const transferReconcileIntervalMs = 2_000;

// What SFTP or Home asked of this machine's terminals while they ran on a run
// target: a session to open there, or one Home opened there to show.
type PendingLocalRequest = { kind: "open"; alias: string; cwd?: string } | { kind: "show"; sessionId: string };

type AppProps = {
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  vault?: () => Promise<PasswordVaultStatus>;
};

// The shell around every section: the session with the engine, the frame
// (header, navigation, inspector, command palette), the terminal sessions that stay
// alive behind the sections, and what one section hands to the next.
export function App({
  bootstrap,
  health,
  vault = vaultApi.passwordVault,
}: AppProps) {
  const { t } = useLanguage();
  useAppViewport();
  useFieldLetGoOnLeaving();
  const mobileLayout = useMediaQuery(mobileViewportQuery);
  const navigationPinned = useMediaQuery(pinnedNavigationQuery);
  const navigationRail = useMediaQuery(navigationRailQuery);
  const shortcuts = useBindings();
  const { resolved: resolvedTheme, scheme } = useTheme();
  const { route, location, navigate, navigateLocation, setNavigationBlocker, hasEntryBefore, currentLocation } =
    useSectionRoute({ present: presentLocationChange });
  const section = route.kind === "section" ? route.section : null;
  const terminalFace = section === "Terminal";
  const session = useAppSession({ bootstrap, health, vault });
  const {
    state,
    vaultRecheck,
    failure,
    vaultExists,
    version,
    requestFailure,
    vaultMigration,
  } = session;
  usePresetLibrarySync(state === "ready");
  // The settings come with the engine being ready; the shell waits for them (settingsLoaded) so the screens
  // open with the widths and views this machine chose rather than the defaults first.
  const settingsLoaded = useSettingsSync(state === "ready");
  const keptSections = useKeptSections({ shown: section, setNavigationBlocker });
  usePreloadedSectionPages(state === "ready");
  useFirstScreenReport(state !== "starting" && (state !== "ready" || settingsLoaded));
  const handoffs = useSectionHandoffs(navigate);
  const declared = useDeclaredConfig(state === "ready", section);
  const [desktopNavigationWidth, resizeDesktopNavigation] = useColumnWidth(navigationWidth);
  const [desktopNavigation, setDesktopNavigation] = useStoredValue(navigationShown);
  const {
    navigationOpen,
    setNavigationOpen,
    closeNavigation,
    navigationPanelRef,
    navigationTriggerRef,
    inspector,
    setInspector,
    inspectorOpen,
    setInspectorOpen,
    inspectorPanelRef,
    inspectorTriggerRef,
    commandPaletteOpen,
    closePalette,
    commandPaletteReturnFocusRef,
    openPalette,
    openPaletteFromNavigation,
  } = useShellLayers({ mobileLayout, ready: state === "ready", section, onBackWithoutLayer: goBackOnAndroid });
  // On the top of Home, Back leaves the app: Android takes it there and draws its own way home (backOwnership.ts).
  // Anywhere else, a Back that goes to another page shrinks the page under the finger (backGestureMotion.ts).
  const onTopOfHome = location.pathname === "/" && location.search === "";
  useBackOwnership(onTopOfHome);
  const backMovesPage = useCallback(
    () => state === "ready" && !onTopOfHome && !somethingForBackToClose(),
    [state, onTopOfHome],
  );
  useBackGestureMotion(backMovesPage);

  // Android's Back once nothing is left to close: Home from the top of a section, one step back inside one
  // (androidBackStep). Leaving the app and going back in the page's history are left to the app (MainActivity).
  // A Back that comes while the page is still changing goes on from the new page, not from the one still drawn.
  function goBackOnAndroid(event: Event) {
    if (state !== "ready") return;
    const step = androidBackStep(currentLocation(), hasEntryBefore());
    if (step.kind !== "section") return;
    event.preventDefault();
    navigateLocation(sectionPath(step.section), { replace: true });
  }

  // Two themes of the same scheme have different bars, so this follows the theme, not only its scheme.
  useEffect(() => {
    setAndroidAppearance(scheme);
  }, [resolvedTheme, scheme]);

  const runTargets = useRunTargets({ translate: t, enabled: state === "ready", localVersion: version });
  const runTargetApis = useRunTargetApis(runTargets.selected, runTargets.vaultUnlock.revision);
  const sessionAddresses = useMemo(
    () => runTargets.selected === null ? addressesByAlias(declared.hosts) : new Map<string, string>(),
    [declared.hosts, runTargets.selected],
  );
  const terminalSessions = useTerminalSessions(runTargetApis.sessions, t, state === "ready");
  const runTargetLink = useRunTargetLink({
    selected: runTargets.selected,
    api: runTargets.api,
    enabled: state === "ready" && runTargets.engines.length > 0,
    loaded: terminalSessions.loaded,
    refusal: terminalSessions.remoteRefusal,
  });
  const terminalWorkspaces = useTerminalWorkspaces({
    sessions: terminalSessions.sessions,
    translate: t,
    enabled: state === "ready",
    onSessionsMoved: terminalSessions.refresh,
    api: runTargetApis.workspaces,
  });
  const terminalWorkspace = useTerminalWorkspaceController({
    runTarget: runTargets.selected,
    terminalSessions,
    workspaces: terminalWorkspaces.workspaces,
    section,
    navigate,
    closeNavigation,
    vaultRevision: runTargets.vaultUnlock.revision,
  });
  const {
    splitView,
    activeSessionId,
    terminalScreenMounted,
    orderedSessions,
    showSession,
    openLocalShell,
    openSSHSession,
    openSSHSessions,
    duplicateSession,
    openSessionInSplit,
    reorderSessions: setSessionOrder,
  } = terminalWorkspace;
  const directDestination = useDirectDestinationActions({
    aliases: declared.knownAliases,
    canSave: runTargets.selected === null,
    open: openSSHSession,
    onSaved: declared.readAgain,
  });
  // The terminals draw with the preset this machine uses and the global settings.
  const terminalSettings = useTerminalSettings();
  // A phone shows the terminal's own top bar instead of the app header and the bottom tabs only
  // while a session can be used: with none open, or while the run target refuses (the panel makes
  // the terminal inert), the app header keeps the navigation reachable.
  const terminalTopBar = mobileLayout && terminalFace && activeSessionId !== null && terminalSessions.remoteRefusal === null;
  const mobileNavigationShown = state === "ready" && mobileLayout && !terminalTopBar;
  // What sits above the floating bottom bar outside the app's own tree (the toast region) reads its height
  // from the root element too (--mobile-navigation-inset in index.css).
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.mobileNavigation = String(mobileNavigationShown);
    root.dataset.mobileNavigationPlacement = navigationRail ? "rail" : "bottom";
    return () => {
      delete root.dataset.mobileNavigation;
      delete root.dataset.mobileNavigationPlacement;
    };
  }, [mobileNavigationShown, navigationRail]);
  // A navigation kept open at the left is not a layer: turning a phone into that shape closes the drawer it had
  // open, so no backdrop or focus trap is left over it.
  useEffect(() => {
    if (navigationPinned) setNavigationOpen(false);
  }, [navigationPinned, setNavigationOpen]);
  const navigationBackdropRef = useRef<HTMLDivElement>(null);
  useNavigationSwipe({ enabled: mobileLayout && !navigationPinned && state === "ready", open: navigationOpen, panelRef: navigationPanelRef, backdropRef: navigationBackdropRef, onOpenChange: setNavigationOpen });

  // A page's heading shows the inspector's toggle among its actions on a wide screen, and puts its actions
  // in the top bar on a phone, where its name is hidden (see pageChrome).
  const [phoneActionsSlot, setPhoneActionsSlot] = useState<HTMLDivElement | null>(null);
  const [phoneBackSlot, setPhoneBackSlot] = useState<HTMLDivElement | null>(null);
  const pageChrome = useMemo<PageChrome>(() => ({
    inspectorToggle: inspector === null || mobileLayout ? null : (
      <span className="shrink-0 [&>button]:h-8">
        <InspectorToggle
          label={inspector.label}
          open={inspectorOpen}
          attention={inspector.attention}
          buttonRef={inspectorTriggerRef}
          onToggle={() => setInspectorOpen((open) => !open)}
        />
      </span>
    ),
    phoneActionsSlot: mobileLayout ? phoneActionsSlot : null,
    phoneBackSlot: mobileLayout ? phoneBackSlot : null,
  }), [inspector, inspectorOpen, inspectorTriggerRef, mobileLayout, phoneActionsSlot, phoneBackSlot, setInspectorOpen]);

  // SFTP and Home open terminals on this machine. With an engine on an SSH
  // host chosen, the run target goes back to this machine first, and what was
  // asked for is done once this machine's terminal API is the one in use.
  const [pendingLocalRequest, setPendingLocalRequest] = useState<PendingLocalRequest | null>(null);
  const [sessionShortcut, setSessionShortcut] = useState<SessionShortcutRequest | null>(null);
  const requestSession = useCallback((request: SessionShortcut) => {
    setSessionShortcut((previous) => ({ ...request, serial: (previous?.serial ?? 0) + 1 }));
  }, []);
  const panes = useMemo(() => paneSessionIds(splitView.state.root), [splitView.state.root]);
  // A PC hides its navigation on the shortcut, keeping a narrow column with the button that shows it again; a
  // phone opens or closes its drawer, and a tablet held upright keeps its navigation open.
  const navigationHidden = !mobileLayout && desktopNavigation === "hidden";
  const toggleNavigation = useCallback(() => {
    if (!mobileLayout) setDesktopNavigation(desktopNavigation === "hidden" ? "shown" : "hidden");
    else if (!navigationPinned) setNavigationOpen((open) => !open);
  }, [mobileLayout, navigationPinned, desktopNavigation, setDesktopNavigation, setNavigationOpen]);
  const openSFTPTerminal = useCallback(async (alias: string, cwd?: string) => {
    if (runTargets.selected !== null) {
      setPendingLocalRequest(cwd === undefined ? { kind: "open", alias } : { kind: "open", alias, cwd });
      runTargets.select(null);
      return;
    }
    if (alias === localHostAlias) {
      await openLocalShell(undefined, cwd);
      return;
    }
    await openSSHSession(alias, cwd);
  }, [openLocalShell, openSSHSession, runTargets]);

  // Home opens a session on this machine itself, so it is shown there.
  const showLocalSession = useCallback((id: string) => {
    if (runTargets.selected !== null) {
      setPendingLocalRequest({ kind: "show", sessionId: id });
      runTargets.select(null);
      return;
    }
    showSession(id);
  }, [runTargets, showSession]);

  useEffect(() => {
    if (pendingLocalRequest === null || runTargets.selected !== null) return;
    setPendingLocalRequest(null);
    if (pendingLocalRequest.kind === "show") showSession(pendingLocalRequest.sessionId);
    else void openSFTPTerminal(pendingLocalRequest.alias, pendingLocalRequest.cwd);
  }, [openSFTPTerminal, pendingLocalRequest, runTargets.selected, showSession]);

  // Splitting beside the session shown: a phone shows one pane at a time, and has no splitting to ask for.
  const splittableSessionId = !splitView.compact && activeSessionId !== null && splitView.splitRefusal(activeSessionId) === null ? activeSessionId : null;
  const { requestSplit } = splitView;
  const splitBesideActive = useMemo(() => splittableSessionId === null
    ? null
    : (edge: PaneEdge) => requestSplit({ targetSessionId: splittableSessionId, edge }), [requestSplit, splittableSessionId]);

  const appActionContext: AppActionContext = {
    terminalFace,
    orderedSessions,
    activeSessionId,
    panes,
    splitBesideActive,
    navigate,
    showSession,
    openPalette,
    requestSession,
    toggleNavigation,
  };
  useAppShortcuts({ enabled: state === "ready", shortcuts, ...appActionContext });

  // Transfers run in the engine, so the queue keeps up even while this tab is
  // hidden; a couple of seconds is fast enough for progress and cheap enough
  // for the engine.
  usePolling(() => sftpTransferManager.reconcile(), {
    intervalMs: transferReconcileIntervalMs, enabled: state === "ready", whileHidden: true, immediately: true,
  });
  useTransferUnloadWarning();

  const unreadSessions = useTerminalNotifications(
    terminalSessions.sessions,
    terminalFace ? activeSessionId : null,
    t,
    showSession,
  );
  useBlockedProgramNotifications(terminalSessions.sessions, {
    enabled: !notificationsComeFromAndroid() || runTargets.selected !== null,
    t,
    onOpenSession: showSession,
  });
  const seenProgramStatus = useSeenProgramStatus(terminalSessions.sessions, terminalFace ? activeSessionId : null);
  useUnseenBlockedProgramNotifications(terminalSessions.sessions, seenProgramStatus, { t, onOpenSession: showSession });


  function addConnection() {
    handoffs.setConnectionDraft(blankConnectionDraft);
    navigate("Connections");
  }

  const changeOSC52 = useOSC52Setting();
  const paletteCommands = usePaletteCommands({
    passwordless: session.passwordless,
    bindings: shortcuts,
    actionContext: appActionContext,
    removeActiveFromSplit: activeSessionId !== null && splitView.groupBySession.has(activeSessionId) ? () => splitView.removeFromSplit(activeSessionId) : null,
    osc52Enabled: terminalSettings.osc52 ?? false,
    onOSC52Change: changeOSC52,
    navigate,
    addConnection,
    openLocalShell: () => void openLocalShell(),
    onLocked: session.lock,
    onStillUnlocked: session.openVault,
  });

  function followSectionLink(
    event: MouseEvent<HTMLAnchorElement>,
    target: Section,
  ) {
    if (!opensInThisPage(event)) return;
    event.preventDefault();
    navigate(target);
  }

  function navigateFromMenu(event: MouseEvent<HTMLAnchorElement>, target: Section) {
    setNavigationOpen(false);
    followSectionLink(event, target);
  }

  // While the vault is rechecked on coming back to the page, the app is inert; the focus goes back to the terminal
  // (or whatever had it) afterwards, but not to a text field on a phone, where it would raise the keyboard.
  const heldForRecheck = state === "ready" && vaultRecheck !== "idle";
  const rechecked = useRef<HTMLDivElement>(null);
  useFocusBackAfterInert(rechecked, heldForRecheck, { keyboardOnScreen: mobileLayout });

  if (state === "locked") {
    return (
      <LockScreen
        exists={vaultExists}
        passwordless={session.passwordless}
        minPassphraseLength={session.minPassphraseLength}
        version={version}
        offerSignOut={!runsInAndroidApp()}
        onExists={session.markVaultExists}
        onOpen={session.openVault}
      />
    );
  }

  if (state === "error") {
    return (
      <BootstrapErrorScreen
        failure={failure}
        requestFailure={requestFailure}
        version={version}
        onCloseFailure={session.clearRequestFailure}
      />
    );
  }

  if (state === "session-ended") return <SessionEndedScreen />;

  const app = (
    <div className="sshc-app flex h-screen flex-col bg-canvas text-ink" data-mobile={mobileLayout} data-navigation-pinned={mobileLayout && navigationPinned} data-navigation-hidden={navigationHidden}>
      <div
        ref={rechecked}
        className="contents"
        inert={heldForRecheck}
        aria-hidden={heldForRecheck ? true : undefined}
      >
        {/* On a phone the terminal draws its own top bar (its menu button opens this navigation), as in the design. */}
        {terminalTopBar ? null : <AppHeader
          route={route}
          navigationOpen={navigationOpen}
          navigationId={navigationId}
          navigationToggleRef={navigationTriggerRef}
          navigationPinned={navigationPinned}
          onToggleNavigation={() => setNavigationOpen((open) => !open)}
          inspector={inspector}
          inspectorOpen={inspectorOpen}
          {...(mobileLayout ? { inspectorToggleRef: inspectorTriggerRef } : {})}
          onToggleInspector={() => setInspectorOpen((open) => !open)}
          actionsSlotRef={setPhoneActionsSlot}
          backSlotRef={setPhoneBackSlot}
          sectionLabels={sectionLabels}
        />}
        {requestFailure === null ? null : (
          <ErrorDiagnosticNotice
            diagnostic={requestFailure}
            version={version}
            onClose={session.clearRequestFailure}
          />
        )}
        <div
          style={
            {
              "--navigation-width": navigationHidden ? hiddenNavigationWidth : `${desktopNavigationWidth}px`,
            } as CSSProperties
          }
          className={`sshc-app-layout grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[var(--navigation-width)_minmax(0,1fr)] ${
            inspector !== null && inspectorOpen
              ? "lg:grid-cols-[var(--navigation-width)_minmax(0,1fr)_17rem]"
              : ""
          }`}
        >
          {/* The backdrop stays in the page while the navigation is closed, unseen and not in the way, so that a finger
              pulling the navigation out darkens it as far as the panel is out (useNavigationSwipe). It fades with the
              panel's own slide when a button opens or closes the navigation. */}
          {navigationPinned ? null : (
            <div
              ref={navigationBackdropRef}
              aria-hidden="true"
              data-navigation-backdrop
              onClick={() => setNavigationOpen(false)}
              className={`sshc-navigation-backdrop fixed inset-0 z-20 bg-canvas/70 transition-opacity motion-reduce:transition-none md:hidden ${navigationOpen ? "opacity-100" : "pointer-events-none opacity-0"}`}
            />
          )}
          {navigationHidden ? (
            // The column the hidden navigation leaves (hiddenNavigationWidth): only the button that shows it again.
            <div className="flex justify-center pt-3">
              <button
                type="button"
                aria-label={t("shell.navigationShow")}
                title={t("shell.navigationShow")}
                onClick={toggleNavigation}
                className="grid size-8 place-items-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
              >
                <Icon name="navigationShow" className="size-4" />
              </button>
            </div>
          ) : null}
          <AppNavigation
            navigationRef={navigationPanelRef}
            navigationId={navigationId}
            version={version}
            state={state}
            navigationOpen={navigationOpen || navigationPinned}
            mobileLayout={mobileLayout}
            desktopWidth={desktopNavigationWidth}
            onDesktopWidthChange={resizeDesktopNavigation}
            startSections={startSections}
            section={section}
            sectionIcons={sectionIcons}
            sectionLabels={sectionLabels}
            onNavigate={navigateFromMenu}
            terminalSessions={terminalSessions}
            terminalWorkspaces={terminalWorkspaces}
            runTargets={runTargets}
            runTargetLink={runTargetLink}
            orderedSessions={orderedSessions}
            activeSessionId={activeSessionId}
            unreadBySession={unreadSessions}
            seenProgramStatus={seenProgramStatus}
            onShowSession={showSession}
            splitView={splitView}
            onShowSplitGroup={(number) => {
              splitView.dispatch({ type: "select-group", number });
              closeNavigation();
              navigate("Terminal");
            }}
            onShowTerminal={() => {
              closeNavigation();
              navigate("Terminal");
            }}
            canSplitSession={(id) => id === activeSessionId ? splitBesideActive !== null : splitView.canSplit(id)}
            onSplitSession={(id, edge) => {
              if (activeSessionId === null) return;
              // The session shown cannot go beside itself: this asks what goes beside it, as the terminal's 「…」 does.
              if (id === activeSessionId) {
                splitBesideActive?.(edge);
                return;
              }
              splitView.split({ sourceSessionId: id, targetSessionId: activeSessionId, edge });
              closeNavigation();
              navigate("Terminal");
            }}
            onDuplicateSession={(id) => void duplicateSession(id)}
            onReorderSessions={setSessionOrder}
            onOpenShell={(workspaceId) => void openLocalShell(undefined, undefined, workspaceId)}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            onConnect={(alias, workspaceId) => void openSSHSession(alias, undefined, workspaceId)}
            directDestination={directDestination}
            onOpenCommandPalette={openPaletteFromNavigation}
            sessionShortcut={sessionShortcut}
          />

          <main className="relative flex min-h-0 min-w-0 flex-col overflow-hidden">
            <SectionNotices
              section={section}
              vaultMigration={vaultMigration}
              onDismissVaultMigration={session.clearVaultMigration}
              connectionDraft={handoffs.connectionDraft}
              onReturnToConnectionDraft={() => navigate("Connections")}
            />
            {state === "ready" && settingsLoaded ? (
              <div className="relative min-h-0 flex-1 overflow-hidden">
                {terminalScreenMounted ? (
                  <div className={terminalFace ? "h-full" : "hidden"}>
                    <TerminalScreen
                      visible={terminalFace}
                      splitView={splitView}
                      terminalSessions={terminalSessions}
                      activeSessionId={activeSessionId}
                      settings={terminalSettings}
                      hostAppearance={declared.hostAppearance}
                      hostVPN={declared.hostVPN}
                      api={runTargetApis.sessions}
                      runTargets={runTargets}
                      runTargetLink={runTargetLink}
                      onOSC52Change={changeOSC52}
                      onOpenRemotePath={handoffs.openRemotePath}
                      onOpenNavigation={navigationPinned ? undefined : () => setNavigationOpen(true)}
                      onNewSession={() => requestSession({ action: "newSession" })}
                      onAddConnection={addConnection}
                      onOpenSync={() => navigate("Sync")}
                      onOpenConnection={(alias) => void openSSHSession(alias)}
                      onShowConnections={() => navigate("Connections")}
                    />
                  </div>
                ) : null}
                <PageChromeContext.Provider value={pageChrome}>
                  <KeptSections
                    kept={keptSections.kept}
                    shown={section}
                    viewProps={{
                      navigation: {
                        location,
                        fileTarget: handoffs.fileTarget,
                        onFileTargetHandled: handoffs.consumeFileTarget,
                        onNavigate: navigate,
                        onNavigateLocation: navigateLocation,
                        onNavigateForCreation: (target: CreationPrerequisite) =>
                          navigate(target),
                        onOpenFile: handoffs.openFile,
                        onNavigationBlockerChange: keptSections.setNavigationBlocker,
                        onOpenSearch: openPalette,
                      },
                      handoff: {
                        connectionKey: handoffs.connectionKey,
                        publicKey: handoffs.publicKey,
                        connectionDraft: handoffs.connectionDraft,
                        onAssignGeneratedKey: handoffs.assignGeneratedKey,
                        onInstallGeneratedKey: handoffs.installGeneratedKey,
                        onConnectionKeyApplied: handoffs.consumeConnectionKey,
                        onPublicKeyHandled: handoffs.consumePublicKey,
                        onConnectionDraftChange: handoffs.setConnectionDraft,
                      },
                      shell: {
                        onLock: session.lock,
                        onVaultChanged: session.openVault,
                        passwordless: session.passwordless,
                        onInspector: setInspector,
                        onShowInspector: () => setInspectorOpen(true),
                        terminalSessions,
                        onShowSession: showLocalSession,
                        onOpenSSHSession: openSSHSession,
                        onOpenSSHSessions: openSSHSessions,
                        onOverviewRead: declared.adoptOverview,
                        onOpenLocalShell: () => void openLocalShell(),
                        onAddConnection: addConnection,
                        onOpenTerminal: openSFTPTerminal,
                      },
                      declared: { groups: declared.groups, knownAliases: declared.knownAliases, hosts: declared.hosts, hostVPN: declared.hostVPN },
                      sftpTarget: handoffs.sftpTarget,
                      onSftpTargetHandled: handoffs.consumeSftpTarget,
                    }}
                  />
                </PageChromeContext.Provider>
                {route.kind === "section" ? null : (
                  <NotFoundSection pathname={route.pathname} onGoHome={(event) => followSectionLink(event, "Home")} />
                )}
              </div>
            ) : <RouteSkeleton />}
          </main>
          {inspector !== null && inspectorOpen ? (
            <InspectorPane label={inspector.label} underTopBar={mobileLayout && !terminalTopBar} onClose={() => setInspectorOpen(false)} paneRef={inspectorPanelRef}>
              {inspector.body}
            </InspectorPane>
          ) : null}
        </div>
        {mobileNavigationShown ? <MobileNavigation section={section} onNavigate={navigateFromMenu} /> : null}
        {state === "ready" ? <TransferNotifications /> : null}
        {/* Asked of the engine in use: a run target asks the first time it is used after
            it restarted, once its Vault is unlocked (it lists its sessions again). An
            engine that restarted between two lists names a new offer, which asks again. */}
        {state === "ready" && terminalSessions.remoteRefusal === null ? (
          <TerminalRestorePrompt
            key={`${runTargets.selected ?? ""}:${terminalSessions.restoreOffer ?? ""}`}
            api={runTargetApis.restore}
            host={runTargets.selected}
            onRestored={terminalSessions.refresh}
          />
        ) : null}
        {state === "ready" ? (
          <SplitRequestDialog
            splitView={splitView}
            sessions={terminalSessions.sessions}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            runTarget={runTargets.selected}
            directDestination={directDestination}
            onOpen={(request, place) => void openSessionInSplit(request, place)}
            onSplit={() => navigate("Terminal")}
          />
        ) : null}
        {state === "ready" ? (
          <CommandPalette
            open={commandPaletteOpen}
            commands={paletteCommands.commands}
            laterCommands={paletteCommands.laterCommands}
            returnFocusRef={commandPaletteReturnFocusRef}
            hosts={declared.hosts}
            files={declared.files}
            sessions={orderedSessions}
            unreadBySession={unreadSessions}
            sectionLabels={sectionLabels}
            onClose={closePalette}
            onConnect={openSSHSession}
            direct={directDestination}
            onOpenHostSettings={(identity) =>
              navigateLocation(
                connectionLocation({
                  path: identity.path,
                  alias: identity.alias,
                  panel: "Basic",
                  advanced: "Settings",
                }),
              )
            }
            onOpenFile={(path) => handoffs.openFile(path, 1)}
            onNavigate={navigate}
            onOpenSnippet={(id) =>
              navigateLocation(
                `${sectionPath("Snippets")}?snippet=${encodeURIComponent(id)}`,
              )
            }
            onOpenSession={showSession}
          />
        ) : null}
      </div>
      {heldForRecheck && vaultRecheck === "checking" ? <VaultRecheckMark /> : null}
      {heldForRecheck && vaultRecheck === "retrying" ? <VaultRecheckOverlay /> : null}
    </div>
  );
  return <SessionAddressesContext.Provider value={sessionAddresses}>{app}</SessionAddressesContext.Provider>;
}

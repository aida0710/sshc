import { useBindings } from "./keyconfig/presets";
import { usePresetLibrarySync } from "./settings/presetLibrary";
import { presetInUse, useSettings, useSettingsSync } from "./settings/settingsStore";
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
import { configApi } from "./api/config";
import { EngineApisProvider, useEngineApisOf } from "./shell/engineApis";
import { RemoteSessionFrame, remoteSessionTagRoom } from "./shell/RemoteSessionFrame";
import { hiddenNavigationWidth, navigationShown, navigationWidth } from "./shell/navigationLayout";
import { useColumnWidth } from "./ui/useColumnWidth";
import { useStoredValue } from "./ui/useStoredValue";
import { useSectionRoute } from "./routing/useSectionRoute";
import { useTerminalSessions } from "./terminal/sessions";
import { useTerminalWorkspaces } from "./terminal/useTerminalWorkspaces";
import { SessionAddressesContext, SessionEngineContext } from "./terminal/sessionDestination";
import { useRunTargetApis } from "./terminal/useRunTargetApis";
import { useSessionProblemToast } from "./terminal/useSessionProblemToast";
import { useRunTargetLink } from "./terminal/useRunTargetLink";
import { useRunTargets } from "./terminal/useRunTargets";
import { TransferNotifications } from "./sftp/TransferNotifications";
import { transferManagersToFollow } from "./sftp/transferManager";
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
import { useHomeWithoutSessions } from "./shell/useHomeWithoutSessions";
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
import { chooseRunTarget, runTargetChoices } from "./shell/runTargetChoice";
import { remoteSessionColorOf, remoteSessionFrameColor } from "./theme/remoteSessionColor";
import type { HostIdentity } from "./api/config";
import { paneSessionIds, type PaneEdge } from "./terminal/splitView/layout";
import { SplitRequestDialog } from "./terminal/splitView/SplitRequestDialog";
import { TerminalRestorePrompt } from "./terminal/TerminalRestorePrompt";
import { useFocusBackAfterInert } from "./ui/useFocusBackAfterInert";
import { Icon } from "./ui/icons";
import { IconButton } from "./ui/IconButton";

const transferReconcileIntervalMs = 2_000;

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
  const { route, location, navigate, navigateLocation, mayLeaveFor, setNavigationBlocker, hasEntryBefore, currentLocation } =
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
  const keptSections = useKeptSections({ shown: section, setNavigationBlocker });
  usePreloadedSectionPages(state === "ready");
  const handoffs = useSectionHandoffs(navigate);
  // Choosing another engine makes its screens anew. A screen with an unsaved draft is asked first, as when leaving it
  // for the terminal.
  const mayChangeEngine = useCallback(() => mayLeaveFor("Terminal"), [mayLeaveFor]);
  const runTargets = useRunTargets({ translate: t, enabled: state === "ready", localVersion: version, mayChange: mayChangeEngine });
  // 設定はエンジンが使えるようになってから読む。画面は、選んでいるリモートセッションのエンジンの設定（キー設定の
  // プリセットも）で描く。アプリの枠は設定を待つ（settingsLoaded）ので、既定の幅や表示で先に開かない。
  usePresetLibrarySync(state === "ready", runTargets.selected);
  const settingsLoaded = useSettingsSync(state === "ready", runTargets.selected);
  useFirstScreenReport(state !== "starting" && (state !== "ready" || settingsLoaded));
  // The screens work with the engine chosen for the remote session: what the SSH configuration declares, too.
  const engineApis = useEngineApisOf(runTargets.selected, runTargets.vaultUnlock.revision);
  const declared = useDeclaredConfig(state === "ready", section, engineApis.config);
  // リモートセッションに足す sshc エンジンを選ぶ一覧と、リモートセッションの枠の色は、このマシンの接続先を使う。
  // このマシンを選んでいる間は、それは一度読んだ `declared` である。
  const declaredHere = useDeclaredConfig(state === "ready" && runTargets.selected !== null, section, configApi);
  const thisMachineDeclared = runTargets.selected === null ? declared : declaredHere;
  // リモートセッションの枠と名札の色。その接続先（このマシンの Connections）の色、設定（Appearance。画面が描く
  // リモートセッションのエンジンの設定）の色の順で、どちらも無ければさし色。
  const settingsDocument = useSettings().document;
  const remoteFrameColor = runTargets.selected === null
    ? null
    : remoteSessionFrameColor(thisMachineDeclared.hostColour.get(runTargets.selected), remoteSessionColorOf(presetInUse(settingsDocument)));
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

  const runTargetApis = useRunTargetApis(runTargets.selected, runTargets.vaultUnlock.revision);
  const sessionAddresses = useMemo(
    () => addressesByAlias(declared.hosts),
    [declared.hosts],
  );
  const terminalSessions = useTerminalSessions(runTargetApis.sessions, t, state === "ready");
  useSessionProblemToast(terminalSessions.problem);
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
    showSession,
    openLocalShell,
    openSSHSession,
    openSSHSessions,
    duplicateSession,
    openSessionInSplit,
  } = terminalWorkspace;
  const showHomeInPlace = useCallback(() => navigateLocation(sectionPath("Home"), { replace: true }), [navigateLocation]);
  useHomeWithoutSessions({ terminalFace, activeSessionId, terminalSessions, showHome: showHomeInPlace });
  const directDestination = useDirectDestinationActions({
    config: engineApis.config,
    aliases: declared.knownAliases,
    canSave: true,
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
  // SFTP works with the engine the terminals run on, so a terminal opened from a folder opens on that engine.
  const openSFTPTerminal = useCallback(async (alias: string, cwd?: string) => {
    if (alias === localHostAlias) {
      await openLocalShell(undefined, cwd);
      return;
    }
    await openSSHSession(alias, cwd);
  }, [openLocalShell, openSSHSession]);

  // Splitting beside the session shown: a phone shows one pane at a time, and has no splitting to ask for.
  // 右と下は、それぞれの側にペインを足す広さがあるかが違うので、別々に決める。
  const splitTargetId = splitView.compact ? null : activeSessionId;
  const canSplitRight = splitTargetId !== null && splitView.splitRefusal(splitTargetId, "right") === null;
  const canSplitBelow = splitTargetId !== null && splitView.splitRefusal(splitTargetId, "bottom") === null;
  const { requestSplit } = splitView;
  const splitBesideActive = useMemo((): Partial<Record<PaneEdge, () => void>> => {
    if (splitTargetId === null) return {};
    const ask = (edge: PaneEdge) => () => requestSplit({ targetSessionId: splitTargetId, edge });
    return { ...(canSplitRight ? { right: ask("right") } : {}), ...(canSplitBelow ? { bottom: ask("bottom") } : {}) };
  }, [requestSplit, splitTargetId, canSplitRight, canSplitBelow]);

  // ターミナルの領域を見せる。リモートセッションを切り替えたときの接続の経過は、そこに出る。
  const showTerminalArea = useCallback(() => {
    closeNavigation();
    navigate("Terminal");
  }, [closeNavigation, navigate]);
  // 選んでいる sshc エンジンの Vault の画面を開く。リモートセッションの Vault が無いときは、そこで作る。
  const openVault = useCallback(() => {
    closeNavigation();
    navigate("Passwords");
  }, [closeNavigation, navigate]);
  const runTargetList = useMemo(() => runTargetChoices(runTargets.engines), [runTargets.engines]);
  // リモートセッションの一覧から、その接続先（このマシンの Connections）の色などの設定を開く。リモートセッションを
  // 選んでいる間、Connections はリモートの接続先を出すので、localhost に戻してから開く。保存していない下書きを残すと
  // 決めて戻れなかったときは、開かない（リモートの同じ名前の接続先を開いてしまわないため）。
  const openThisMachineConnectionSettings = useCallback((identity: HostIdentity) => {
    if (runTargets.selected !== null && !runTargets.select(null)) return;
    closeNavigation();
    navigateLocation(connectionLocation({ path: identity.path, alias: identity.alias, panel: "Sshc", advanced: "Settings" }));
  }, [runTargets, closeNavigation, navigateLocation]);
  const chooseRunTargetHere = useCallback(
    (alias: string | null) => chooseRunTarget({ targets: runTargets, link: runTargetLink, showTerminal: showTerminalArea }, alias),
    [runTargets, runTargetLink, showTerminalArea],
  );

  const appActionContext: AppActionContext = {
    terminalFace,
    orderedSessions: terminalSessions.sessions,
    activeSessionId,
    panes,
    splitBesideActive,
    navigate,
    showSession,
    openPalette,
    requestSession,
    toggleNavigation,
    runTargets: runTargetList,
    selectedRunTarget: runTargets.selected,
    chooseRunTarget: chooseRunTargetHere,
  };
  useAppShortcuts({ enabled: state === "ready", shortcuts, ...appActionContext });

  // Transfers run in the engine, so the queue keeps up even while this tab is
  // hidden; a couple of seconds is fast enough for progress and cheap enough
  // for the engine.
  // The queue of the engine shown is read, and so is that of any other engine this browser is moving a file for:
  // its jobs go on behind, and the next one starts from what the engine lists.
  usePolling(() => Promise.all(transferManagersToFollow(runTargets.selected).map((manager) => manager.reconcile())).then(() => undefined), {
    intervalMs: transferReconcileIntervalMs, enabled: state === "ready", whileHidden: true, immediately: true,
  });
  useTransferUnloadWarning(runTargets.selected);

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
    <EngineApisProvider apis={engineApis}>
    <div className={`sshc-app flex h-screen flex-col bg-canvas text-ink ${runTargets.selected === null ? "" : remoteSessionTagRoom}`} data-mobile={mobileLayout} data-navigation-pinned={mobileLayout && navigationPinned} data-navigation-hidden={navigationHidden}>
      {runTargets.selected === null ? null : <RemoteSessionFrame alias={runTargets.selected} color={remoteFrameColor} />}
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
              <IconButton label={t("shell.navigationShow")} size="panel"
                onClick={toggleNavigation}>
                <Icon name="navigationShow" className="size-4" />
              </IconButton>
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
            onShowTerminal={showTerminalArea}
            canSplitSession={(id, edge) => id === activeSessionId ? splitBesideActive[edge] !== undefined : splitView.canSplit(id, activeSessionId, edge)}
            splitLacksRoom={(id, edge) => activeSessionId !== null
              && (id === activeSessionId ? splitView.splitRefusal(id, edge) === "room" : splitView.lacksRoomFor(activeSessionId, edge, id))}
            onSplitSession={(id, edge) => {
              if (activeSessionId === null) return;
              // The session shown cannot go beside itself: this asks what goes beside it, as the terminal's 「…」 does.
              if (id === activeSessionId) {
                splitBesideActive[edge]?.();
                return;
              }
              splitView.split({ sourceSessionId: id, targetSessionId: activeSessionId, edge });
              closeNavigation();
              navigate("Terminal");
            }}
            onDuplicateSession={(id) => void duplicateSession(id)}
            onOpenShell={(workspaceId) => void openLocalShell(undefined, undefined, workspaceId)}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            engineChoices={{ aliases: thisMachineDeclared.knownAliases, hosts: thisMachineDeclared.hosts }}
            onOpenConnectionSettings={openThisMachineConnectionSettings}
            onOpenVault={openVault}
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
                      hostSettingsRead={declared.read}
                      api={runTargetApis.sessions}
                      runTargets={runTargets}
                      runTargetLink={runTargetLink}
                      onOSC52Change={changeOSC52}
                      onOpenRemotePath={handoffs.openRemotePath}
                      onOpenNavigation={navigationPinned ? undefined : () => setNavigationOpen(true)}
                      onOpenVault={openVault}
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
                        runTarget: runTargets.selected,
                        onOpenSSHSession: openSSHSession,
                        onOpenSSHSessions: openSSHSessions,
                        onOverviewRead: declared.adoptOverview,
                        onOpenLocalShell: () => void openLocalShell(),
                        onAddConnection: addConnection,
                        onOpenTerminal: openSFTPTerminal,
                      },
                      declared: { groups: declared.groups, knownAliases: declared.knownAliases, hosts: declared.hosts, hostVPN: declared.hostVPN, hostLook: declared.hostLook },
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
            sessions={terminalSessions.sessions}
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
    </EngineApisProvider>
  );
  return (
    <SessionAddressesContext.Provider value={sessionAddresses}>
      <SessionEngineContext.Provider value={runTargets.selected}>{app}</SessionEngineContext.Provider>
    </SessionAddressesContext.Provider>
  );
}

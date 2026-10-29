import { usePresetSync } from "./keyconfig/presets";
import { presetInUse, useTerminalPresets, useTerminalPresetSync } from "./settings/terminalPresetLibrary";
import { withPreset } from "./settings/terminalPresets";
import { opensInThisPage } from "./ui/linkClick";
import { useBindings } from "./keyconfig/bindings";
import { Suspense, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { type HealthResponse } from "./api/client";
import { runTargetTerminalSessionsApi, terminalSessionsApi } from "./api/terminalSessions";
import { createTerminalWorkspacesApi, terminalWorkspacesApi } from "./api/terminalWorkspaces";
import { createTerminalRestoreApi, terminalRestoreApi } from "./api/terminalRestore";
import { engineTerminalBase } from "./api/remoteEngines";
import { settingsApi } from "./api/settings";
import { vaultApi, type PasswordVaultStatus } from "./api/vault";
import type { SessionState } from "./session/bootstrap";
import type { CreationPrerequisite } from "./connections/createConnectionDraft";
import { LockScreen } from "./secrets/LockScreen";
import { useLanguage } from "./i18n/context";
import { InspectorPane, InspectorToggle } from "./ui/Inspector";
import { PageChromeContext, type PageChrome } from "./ui/pageChrome";
import { useAccent } from "./theme/accentContext";
import { useTheme } from "./theme/context";
import { usePolling } from "./ui/usePolling";
import { RouteSkeleton } from "./ui/RouteSkeleton";
import { sectionPath, type Section } from "./routing/sectionRoute";
import { connectionLocation } from "./routing/connectionRoute";
import { AppHeader } from "./shell/AppHeader";
import { AppNavigation } from "./shell/AppNavigation";
import { navigationWidth } from "./shell/navigationLayout";
import { useStoredColumnWidth } from "./ui/useStoredColumnWidth";
import { useSectionRoute } from "./routing/useSectionRoute";
import { useTerminalSessions } from "./terminal/sessions";
import { useTerminalWorkspaces } from "./terminal/useTerminalWorkspaces";
import { useRunTargets } from "./terminal/useRunTargets";
import { TransferNotifications } from "./sftp/TransferNotifications";
import { sftpTransferManager } from "./sftp/transferManager";
import { localHostAlias } from "./sftp/localHost";
import { useTransferUnloadWarning } from "./sftp/useTransferUnloadWarning";
import { ErrorDiagnosticNotice } from "./shell/ErrorDiagnosticNotice";
import { CommandPalette } from "./shell/CommandPalette";
import { MobileNavigation } from "./shell/MobileNavigation";
import { useNavigationSwipe } from "./shell/useNavigationSwipe";
import { SectionNotices } from "./shell/SectionNotices";
import { usePaletteCommands } from "./shell/usePaletteCommands";
import { setAndroidAppearance } from "./android/native";
import { useTerminalNotifications } from "./terminal/useTerminalNotifications";
import { useSeenProgramStatus } from "./terminal/useSeenProgramStatus";
import { useAppSession } from "./session/useAppSession";
import { useTerminalWorkspaceController } from "./terminal/useTerminalWorkspaceController";
import { mobileViewportQuery, useMediaQuery } from "./ui/useMediaQuery";
import { useAppViewport } from "./ui/useAppViewport";
import { BootstrapErrorScreen, NotFoundSection, SessionEndedScreen, VaultRecheckOverlay } from "./shell/AppFallbackScreens";
import { SectionView, navigationId, startSections } from "./shell/sections";
import { sectionIcons, sectionLabels } from "./shell/sectionPresentation";
import { TerminalScreen } from "./shell/TerminalScreen";
import { useDeclaredConfig } from "./shell/useDeclaredConfig";
import { useOSC52Setting } from "./shell/useOSC52Setting";
import { useSectionHandoffs } from "./shell/useSectionHandoffs";
import { useShellLayers } from "./shell/useShellLayers";
import { useAppShortcuts } from "./shell/useAppShortcuts";
import { TerminalRestorePrompt } from "./terminal/TerminalRestorePrompt";
import { useFocusBackAfterInert } from "./ui/useFocusBackAfterInert";

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
  const mobileLayout = useMediaQuery(mobileViewportQuery);
  const shortcuts = useBindings();
  const { resolved: resolvedTheme, scheme } = useTheme();
  const { route, location, navigate, navigateLocation, setNavigationBlocker } =
    useSectionRoute();
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
  usePresetSync(state === "ready");
  useTerminalPresetSync(state === "ready");
  const handoffs = useSectionHandoffs(navigate);
  const declared = useDeclaredConfig(state === "ready", section);
  const [desktopNavigationWidth, resizeDesktopNavigation] = useStoredColumnWidth(navigationWidth);
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
  } = useShellLayers({ mobileLayout, ready: state === "ready", section });

  // Two themes of the same scheme have different bars, so this follows the theme, not only its scheme.
  useEffect(() => {
    setAndroidAppearance(scheme);
  }, [resolvedTheme, scheme]);

  const runTargets = useRunTargets({ translate: t, enabled: state === "ready", localVersion: version });
  // The terminal API of the run target: this machine's own, or the one this
  // machine's engine forwards for an engine on an SSH host. A new API is a new
  // engine to the hooks below, which drop what they showed and read it anew;
  // that is wanted once a run target's Vault is unlocked, so the API is built
  // again for each unlock (vaultRevision).
  const vaultRevision = runTargets.vaultUnlock.revision;
  const runTargetApis = useMemo(() => {
    const alias = runTargets.selected;
    if (alias === null) return { sessions: terminalSessionsApi, workspaces: terminalWorkspacesApi, restore: terminalRestoreApi, vaultRevision };
    return {
      sessions: runTargetTerminalSessionsApi(alias),
      workspaces: createTerminalWorkspacesApi(engineTerminalBase(alias)),
      restore: createTerminalRestoreApi(engineTerminalBase(alias)),
      vaultRevision,
    };
  }, [runTargets.selected, vaultRevision]);
  // The accent is kept in this machine's metadata, which can be read once the engine is ready.
  const { loadAccent } = useAccent();
  useEffect(() => {
    if (state === "ready") loadAccent();
  }, [state, loadAccent]);
  const terminalSessions = useTerminalSessions(runTargetApis.sessions, t, state === "ready");
  const terminalWorkspaces = useTerminalWorkspaces({
    sessions: terminalSessions.sessions,
    translate: t,
    enabled: state === "ready",
    onSessionsMoved: terminalSessions.refresh,
    api: runTargetApis.workspaces,
  });
  const terminalWorkspace = useTerminalWorkspaceController({
    runTarget: runTargets.selected,
    api: settingsApi,
    shells: runTargetApis.sessions,
    terminalSessions,
    workspaces: terminalWorkspaces.workspaces,
    enabled: state === "ready",
    section,
    navigate,
    closeNavigation,
  });
  const {
    splitView,
    settings: terminalSettings,
    localShellProfiles,
    activeSessionId,
    terminalScreenMounted,
    orderedSessions,
    showSession,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    reorderSessions: setSessionOrder,
    setSettings: setTerminalSettings,
  } = terminalWorkspace;
  // The terminals draw with the preset this browser uses. Saving the settings (the OSC 52 switch) keeps to
  // terminalSettings, so a preset's values never end up in the standard set.
  const terminalPresets = useTerminalPresets();
  const appliedTerminalSettings = useMemo(
    () => withPreset(terminalSettings, presetInUse(terminalPresets)),
    [terminalSettings, terminalPresets],
  );
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
    return () => {
      delete root.dataset.mobileNavigation;
    };
  }, [mobileNavigationShown]);
  useNavigationSwipe({ enabled: mobileLayout && state === "ready", open: navigationOpen, panelRef: navigationPanelRef, onOpenChange: setNavigationOpen });

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

  useAppShortcuts({
    enabled: state === "ready",
    shortcuts,
    terminalFace,
    orderedSessions,
    activeSessionId,
    navigate,
    showSession,
    openPalette,
  });

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
  const seenProgramStatus = useSeenProgramStatus(terminalSessions.sessions, terminalFace ? activeSessionId : null);


  const paletteCommands = usePaletteCommands({
    passwordless: session.passwordless,
    navigate,
    startConnectionDraft: handoffs.setConnectionDraft,
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

  const changeOSC52 = useOSC52Setting({ settings: terminalSettings, setSettings: setTerminalSettings });

  // While the vault is rechecked on coming back to the page, the app is hidden and inert; the focus goes back
  // to the terminal (or whatever had it) afterwards.
  const hiddenForRecheck = state === "ready" && vaultRecheck !== "idle";
  const rechecked = useRef<HTMLDivElement>(null);
  useFocusBackAfterInert(rechecked, hiddenForRecheck);

  if (state === "locked") {
    return (
      <LockScreen
        exists={vaultExists}
        passwordless={session.passwordless}
        minPassphraseLength={session.minPassphraseLength}
        version={version}
        offerSignOut
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

  return (
    <div className="sshc-app flex h-screen flex-col bg-canvas text-ink" data-mobile={mobileLayout}>
      <div
        ref={rechecked}
        className="contents"
        inert={hiddenForRecheck}
        aria-hidden={hiddenForRecheck ? true : undefined}
      >
        {/* On a phone the terminal draws its own top bar (‹ opens this navigation), as in the design. */}
        {terminalTopBar ? null : <AppHeader
          route={route}
          navigationOpen={navigationOpen}
          navigationId={navigationId}
          navigationToggleRef={navigationTriggerRef}
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
              "--navigation-width": `${desktopNavigationWidth}px`,
            } as CSSProperties
          }
          className={`sshc-app-layout grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[var(--navigation-width)_minmax(0,1fr)] ${
            inspector !== null && inspectorOpen
              ? "lg:grid-cols-[var(--navigation-width)_minmax(0,1fr)_17rem]"
              : ""
          }`}
        >
          {navigationOpen ? (
            <div
              aria-hidden="true"
              data-navigation-backdrop
              onClick={() => setNavigationOpen(false)}
              className="sshc-navigation-backdrop fixed inset-0 z-20 bg-canvas/70 md:hidden"
            />
          ) : null}
          <AppNavigation
            navigationRef={navigationPanelRef}
            navigationId={navigationId}
            version={version}
            state={state}
            navigationOpen={navigationOpen}
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
            canSplitSession={splitView.canSplit}
            onSplitSession={(id, edge) => {
              if (activeSessionId === null) return;
              splitView.split({ sourceSessionId: id, targetSessionId: activeSessionId, edge });
              closeNavigation();
              navigate("Terminal");
            }}
            onDuplicateSession={(id) => void duplicateSession(id)}
            onReorderSessions={setSessionOrder}
            localShellProfiles={localShellProfiles}
            onOpenShell={(profileId, workspaceId) => void openLocalShell(profileId, undefined, workspaceId)}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            onConnect={(alias, workspaceId) => void openSSHSession(alias, undefined, workspaceId)}
            onOpenCommandPalette={openPaletteFromNavigation}
          />

          <main className="relative flex min-h-0 min-w-0 flex-col overflow-hidden">
            <SectionNotices
              section={section}
              vaultMigration={vaultMigration}
              onDismissVaultMigration={session.clearVaultMigration}
              connectionDraft={handoffs.connectionDraft}
              onReturnToConnectionDraft={() => navigate("Connections")}
            />
            {state === "ready" ? (
              <div className="relative min-h-0 flex-1 overflow-hidden">
                {terminalScreenMounted ? (
                  <div className={terminalFace ? "h-full" : "hidden"}>
                    <TerminalScreen
                      visible={terminalFace}
                      splitView={splitView}
                      terminalSessions={terminalSessions}
                      activeSessionId={activeSessionId}
                      settings={appliedTerminalSettings}
                      hostAppearance={declared.hostAppearance}
                      hostVPN={declared.hostVPN}
                      api={runTargetApis.sessions}
                      runTargets={runTargets}
                      onOSC52Change={changeOSC52}
                      onOpenRemotePath={handoffs.openRemotePath}
                      onOpenNavigation={() => setNavigationOpen(true)}
                    />
                  </div>
                ) : null}
                {route.kind === "section" ? (
                  <PageChromeContext.Provider value={pageChrome}>
                    <Suspense fallback={<RouteSkeleton />}>
                      <SectionView
                        key={route.section}
                        section={route.section}
                        navigation={{
                          location,
                          fileTarget: handoffs.fileTarget,
                          onFileTargetHandled: handoffs.consumeFileTarget,
                          onNavigate: navigate,
                          onNavigateLocation: navigateLocation,
                          onNavigateForCreation: (target: CreationPrerequisite) =>
                            navigate(target),
                          onOpenFile: handoffs.openFile,
                          onNavigationBlockerChange: setNavigationBlocker,
                        }}
                        handoff={{
                          connectionKey: handoffs.connectionKey,
                          publicKey: handoffs.publicKey,
                          connectionDraft: handoffs.connectionDraft,
                          onAssignGeneratedKey: handoffs.assignGeneratedKey,
                          onInstallGeneratedKey: handoffs.installGeneratedKey,
                          onConnectionKeyApplied: handoffs.consumeConnectionKey,
                          onPublicKeyHandled: handoffs.consumePublicKey,
                          onConnectionDraftChange: handoffs.setConnectionDraft,
                        }}
                        shell={{
                          onLock: session.lock,
                          onVaultChanged: session.openVault,
                          passwordless: session.passwordless,
                          onInspector: setInspector,
                          onShowInspector: () => setInspectorOpen(true),
                          terminalSessions,
                          onShowSession: showLocalSession,
                          onOpenSSHSession: openSSHSession,
                          onOpenTerminal: openSFTPTerminal,
                          onTerminalSettingsChange: async (settings) => {
                            setTerminalSettings(settings);
                            await terminalSessions.refresh();
                          },
                        }}
                        declared={{ groups: declared.groups, knownAliases: declared.knownAliases, hosts: declared.hosts, hostVPN: declared.hostVPN }}
                        sftpTarget={handoffs.sftpTarget}
                        onSftpTargetHandled={handoffs.consumeSftpTarget}
                      />
                    </Suspense>
                  </PageChromeContext.Provider>
                ) : (
                  <NotFoundSection pathname={route.pathname} onGoHome={(event) => followSectionLink(event, "Home")} />
                )}
              </div>
            ) : null}
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
          <CommandPalette
            open={commandPaletteOpen}
            commands={paletteCommands}
            returnFocusRef={commandPaletteReturnFocusRef}
            hosts={declared.hosts}
            files={declared.files}
            sessions={orderedSessions}
            unreadBySession={unreadSessions}
            sectionLabels={sectionLabels}
            onClose={closePalette}
            onConnect={openSSHSession}
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
      {hiddenForRecheck ? <VaultRecheckOverlay phase={vaultRecheck} /> : null}
    </div>
  );
}

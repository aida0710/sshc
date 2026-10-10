import { usePresetSync } from "./keyconfig/presets";
import { useBindings } from "./keyconfig/bindings";
import { Suspense, useCallback, useEffect, type CSSProperties, type MouseEvent } from "react";
import { type HealthResponse } from "./api/client";
import { terminalSessionsApi } from "./api/terminalSessions";
import { settingsApi } from "./api/settings";
import { vaultApi, type PasswordVaultStatus } from "./api/vault";
import type { SessionState } from "./session/bootstrap";
import type { CreationPrerequisite } from "./connections/CreateConnectionModal";
import { LockScreen } from "./secrets/LockScreen";
import { useLanguage } from "./i18n/context";
import { InspectorPane, InspectorToggle } from "./ui/Inspector";
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
import { TransferNotifications } from "./sftp/TransferNotifications";
import { sftpTransferManager } from "./sftp/transferManager";
import { localHostAlias } from "./sftp/localHost";
import { useTransferUnloadWarning } from "./sftp/useTransferUnloadWarning";
import { ErrorDiagnosticNotice } from "./shell/ErrorDiagnosticNotice";
import { CommandPalette } from "./shell/CommandPalette";
import { MobileNavigation } from "./shell/MobileNavigation";
import { SectionNotices } from "./shell/SectionNotices";
import { usePaletteCommands } from "./shell/usePaletteCommands";
import { setAndroidAppearance } from "./android/native";
import { useTerminalNotifications } from "./terminal/useTerminalNotifications";
import { useAppSession } from "./session/useAppSession";
import { useTerminalWorkspaceController } from "./terminal/useTerminalWorkspaceController";
import { mobileViewportQuery, useMediaQuery } from "./ui/useMediaQuery";
import { useAppViewport } from "./ui/useAppViewport";
import { BootstrapErrorScreen, NotFoundSection, SessionEndedScreen, VaultRecheckOverlay } from "./shell/AppFallbackScreens";
import { SectionView, navigationId, sectionIcons, sectionLabels, startSections } from "./shell/sections";
import { TerminalScreen } from "./shell/TerminalScreen";
import { useDeclaredConfig } from "./shell/useDeclaredConfig";
import { useOSC52Policy } from "./shell/useOSC52Policy";
import { useSectionHandoffs } from "./shell/useSectionHandoffs";
import { useShellLayers } from "./shell/useShellLayers";
import { useAppShortcuts } from "./shell/useAppShortcuts";

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
  const mobileLayout = useMediaQuery(mobileViewportQuery);
  const shortcuts = useBindings();
  const { resolved: resolvedTheme } = useTheme();
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

  useEffect(() => {
    setAndroidAppearance(resolvedTheme);
  }, [resolvedTheme]);

  const terminalSessions = useTerminalSessions(terminalSessionsApi, t, state === "ready");
  const terminalWorkspace = useTerminalWorkspaceController({
    api: settingsApi,
    terminalSessions,
    enabled: state === "ready",
    section,
    navigate,
    closeNavigation,
  });
  const {
    settings: terminalSettings,
    localShellProfiles,
    activeSessionId,
    terminalScreenMounted,
    liveWorkspace,
    restoreRequest: workspaceRestoreRequest,
    renameRequest: workspaceRenameRequest,
    orderedSessions,
    showSession,
    openWorkspace,
    renameWorkspace,
    openLocalShell,
    openSSHSession,
    duplicateSession,
    consumeRestore: consumeWorkspaceRestore,
    setWorkspaceRestoring,
    consumeRename: consumeWorkspaceRename,
    reorderSessions: setSessionOrder,
    setLiveWorkspace,
    setSettings: setTerminalSettings,
  } = terminalWorkspace;

  const openSFTPTerminal = useCallback(async (alias: string, cwd?: string) => {
    if (alias === localHostAlias) {
      await openLocalShell(undefined, cwd);
      return;
    }
    await openSSHSession(alias, cwd);
  }, [openLocalShell, openSSHSession]);

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
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    navigate(target);
  }

  function navigateFromMenu(event: MouseEvent<HTMLAnchorElement>, target: Section) {
    setNavigationOpen(false);
    followSectionLink(event, target);
  }

  const changeOSC52 = useOSC52Policy({
    settings: terminalSettings,
    setSettings: setTerminalSettings,
    setHostPolicy: declared.setHostOSC52,
  });

  if (state === "locked") {
    return (
      <LockScreen
        exists={vaultExists}
        passwordless={session.passwordless}
        minPassphraseLength={session.minPassphraseLength}
        version={version}
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
        className="contents"
        inert={state === "ready" && vaultRecheck !== "idle"}
        aria-hidden={
          state === "ready" && vaultRecheck !== "idle" ? true : undefined
        }
      >
        <AppHeader
          route={route}
          navigationOpen={navigationOpen}
          navigationId={navigationId}
          navigationToggleRef={navigationTriggerRef}
          onToggleNavigation={() => setNavigationOpen((open) => !open)}
          inspector={inspector}
          inspectorOpen={inspectorOpen}
          inspectorToggleRef={inspectorTriggerRef}
          onToggleInspector={() => setInspectorOpen((open) => !open)}
          sectionLabels={sectionLabels}
        />
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
            orderedSessions={orderedSessions}
            activeSessionId={activeSessionId}
            liveWorkspace={liveWorkspace}
            onRenameWorkspace={renameWorkspace}
            unreadBySession={unreadSessions}
            onShowSession={showSession}
            onDuplicateSession={(id) => void duplicateSession(id)}
            onReorderSessions={setSessionOrder}
            localShellProfiles={localShellProfiles}
            onOpenShell={(profileId) => void openLocalShell(profileId)}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            onConnect={(alias) => void openSSHSession(alias)}
            onOpenCommandPalette={openPaletteFromNavigation}
          />

          <main className="relative flex min-h-0 min-w-0 flex-col overflow-hidden">
            {inspector === null ? null : (
              <span className="absolute right-2 top-2 z-10 hidden md:block [&>button]:h-8 [&>button]:w-8 [&>button]:justify-center [&>button>span]:hidden">
                <InspectorToggle
                  label={inspector.label}
                  open={inspectorOpen}
                  attention={inspector.attention}
                  onToggle={() => setInspectorOpen((open) => !open)}
                />
              </span>
            )}
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
                      terminalSessions={terminalSessions}
                      activeSessionId={activeSessionId}
                      settings={terminalSettings}
                      hostAppearance={declared.hostAppearance}
                      hostOSC52={declared.hostOSC52}
                      hostVPN={declared.hostVPN}
                      onActive={showSession}
                      onLiveWorkspaceChange={setLiveWorkspace}
                      onOpenAlias={(alias) =>
                        terminalSessions.open({ kind: "ssh", alias })
                      }
                      onOpenShell={() => terminalSessions.open({ kind: "shell" })}
                      onOSC52Change={changeOSC52}
                      onOpenRemotePath={handoffs.openRemotePath}
                      restoreRequest={workspaceRestoreRequest}
                      onRestoreConsumed={consumeWorkspaceRestore}
                      onRestoringChange={setWorkspaceRestoring}
                      renameRequest={workspaceRenameRequest}
                      onRenameConsumed={consumeWorkspaceRename}
                    />
                  </div>
                ) : null}
                {route.kind === "section" ? (
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
                        terminalSessions,
                        onShowSession: showSession,
                        onOpenSSHSession: openSSHSession,
                        onOpenTerminal: openSFTPTerminal,
                        onOpenWorkspace: openWorkspace,
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
                ) : (
                  <NotFoundSection pathname={route.pathname} onGoHome={(event) => followSectionLink(event, "Home")} />
                )}
              </div>
            ) : null}
          </main>
          {inspector !== null && inspectorOpen ? (
            <InspectorPane label={inspector.label} paneRef={inspectorPanelRef}>
              {inspector.body}
            </InspectorPane>
          ) : null}
        </div>
        {state === "ready" && mobileLayout ? (
          <MobileNavigation section={section} onNavigate={navigateFromMenu} />
        ) : null}
        {state === "ready" ? <TransferNotifications /> : null}
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
                  advanced: "Jump",
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
      {state === "ready" && vaultRecheck !== "idle" ? <VaultRecheckOverlay phase={vaultRecheck} /> : null}
    </div>
  );
}

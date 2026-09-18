import { usePresetSync } from "./keyconfig/presets";
import { useBindings } from "./keyconfig/bindings";
import {
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react";
import { type HealthResponse } from "./api/client";
import { terminalSessionsApi } from "./api/terminalSessions";
import { settingsApi } from "./api/settings";
import { vaultApi, type PasswordVaultStatus } from "./api/vault";
import type { SessionState } from "./session/bootstrap";
import type { CreationPrerequisite } from "./connections/CreateConnectionModal";
import { LockScreen } from "./secrets/LockScreen";
import { useLanguage } from "./i18n/context";
import { Icon, IconSprite } from "./ui/icons";
import { InspectorPane, InspectorToggle, type InspectorContent } from "./ui/Inspector";
import { useTheme } from "./theme/context";
import { Button } from "./ui/surface";
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
import { ErrorDiagnosticNotice } from "./shell/ErrorDiagnosticNotice";
import { CommandPalette, type PaletteCommand } from "./shell/CommandPalette";
import { setAndroidAppearance } from "./android/native";
import { useTerminalNotifications } from "./terminal/terminalNotifications";
import { useAppSession } from "./session/useAppSession";
import { useTerminalWorkspaceController } from "./terminal/useTerminalWorkspaceController";
import { useDismissibleLayer } from "./ui/useDismissibleLayer";
import { mobileViewportQuery, useMediaQuery } from "./ui/useMediaQuery";
import { useAppViewport } from "./ui/useAppViewport";
import { BootstrapErrorScreen, NotFoundSection, SessionEndedScreen, VaultRecheckOverlay } from "./shell/AppFallbackScreens";
import { SectionView, navigationId, sectionIcons, sectionLabels, startSections } from "./shell/sections";
import { TerminalScreen } from "./shell/TerminalScreen";
import { useDeclaredConfig } from "./shell/useDeclaredConfig";
import { useOSC52Policy } from "./shell/useOSC52Policy";
import { useSectionHandoffs } from "./shell/useSectionHandoffs";
import { useAppShortcuts } from "./shell/useAppShortcuts";

export { vaultStatePollIntervalMs } from "./session/useAppSession";
export { resolveOSC52 } from "./shell/TerminalScreen";

type AppProps = {
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  vault?: () => Promise<PasswordVaultStatus>;
};

// The shell around every section: the session with the engine, the frame
// (header, navigation, inspector, command palette), the consoles that stay
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
  const [navigationOpen, setNavigationOpen] = useState(false);
  const navigationPanelRef = useRef<HTMLElement>(null);
  const navigationTriggerRef = useRef<HTMLButtonElement>(null);
  const [desktopNavigationWidth, resizeDesktopNavigation] = useStoredColumnWidth(navigationWidth);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspector, setInspector] = useState<InspectorContent>(null);
  const inspectorPanelRef = useRef<HTMLElement>(null);
  const inspectorTriggerRef = useRef<HTMLButtonElement>(null);
  const inspectorIsOverlay = useMediaQuery("(max-width: 1023px)");
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const commandPaletteReturnFocusRef = useRef<HTMLElement>(null);

  useDismissibleLayer({
    open: navigationOpen,
    containerRefs: [navigationPanelRef, navigationTriggerRef],
    onDismiss: () => setNavigationOpen(false),
    returnFocusRef: navigationTriggerRef,
    trapFocus: mobileLayout,
  });
  useDismissibleLayer({
    open: inspectorOpen && inspector !== null && inspectorIsOverlay,
    containerRefs: [inspectorPanelRef, inspectorTriggerRef],
    onDismiss: () => setInspectorOpen(false),
    closeOnOutside: false,
    returnFocusRef: inspectorTriggerRef,
    initialFocusRef: inspectorPanelRef,
    trapFocus: true,
  });

  useEffect(() => {
    setAndroidAppearance(resolvedTheme);
  }, [resolvedTheme]);

  useEffect(() => {
    if (state !== "ready") setCommandPaletteOpen(false);
  }, [state]);

  useEffect(() => {
    function closeTransientUi(event: Event) {
      if (commandPaletteOpen) {
        event.preventDefault();
        setCommandPaletteOpen(false);
      } else if (navigationOpen) {
        event.preventDefault();
        setNavigationOpen(false);
      } else if (inspectorOpen) {
        event.preventDefault();
        setInspectorOpen(false);
      }
    }
    window.addEventListener("sshc-android-back", closeTransientUi);
    return () =>
      window.removeEventListener("sshc-android-back", closeTransientUi);
  }, [commandPaletteOpen, inspectorOpen, navigationOpen]);

  const consoles = useTerminalSessions(terminalSessionsApi, t, state === "ready");
  const closeNavigation = useCallback(() => setNavigationOpen(false), []);
  const terminalWorkspace = useTerminalWorkspaceController({
    api: settingsApi,
    consoles,
    enabled: state === "ready",
    section,
    navigate,
    closeNavigation,
  });
  const {
    settings: terminalSettings,
    localShellProfiles,
    activeConsole,
    liveWorkspace,
    restoreRequest: workspaceRestoreRequest,
    renameRequest: workspaceRenameRequest,
    orderedConsoles,
    showConsole,
    openWorkspace,
    renameWorkspace,
    openLocalShell,
    duplicateConsole,
    consumeRestore: consumeWorkspaceRestore,
    consumeRename: consumeWorkspaceRename,
    reorderConsoles: setConsoleOrder,
    setLiveWorkspace,
    setSettings: setTerminalSettings,
  } = terminalWorkspace;

  const openPalette = useCallback(() => {
    commandPaletteReturnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setCommandPaletteOpen(true);
  }, []);
  useAppShortcuts({
    enabled: state === "ready",
    shortcuts,
    terminalFace,
    orderedConsoles,
    activeConsole,
    navigate,
    showConsole,
    openPalette,
  });

  useEffect(() => {
    if (state !== "ready") return;
    const refresh = () => {
      void sftpTransferManager.reconcile().catch(() => undefined);
    };
    refresh();
    const timer = globalThis.setInterval(refresh, 2_000);
    return () => globalThis.clearInterval(timer);
  }, [state]);

  const unreadSessions = useTerminalNotifications(
    consoles.sessions,
    terminalFace ? activeConsole : null,
    t,
    showConsole,
  );

  useEffect(() => {
    setInspector(null);
  }, [section]);


  // Ctrl/Cmd+K performs as well as navigates. These are the actions that are
  // otherwise several clicks deep from wherever the user happens to be.
  const paletteCommands: PaletteCommand[] = [
    {
      id: "new-connection",
      label: t("palette.newConnection"),
      detail: t("palette.newConnectionDetail"),
      search: "new connection create host add 新規 接続 追加 作成",
      run: () => {
        handoffs.setConnectionDraft({
          alias: "", group: "", hostName: "", user: "", port: "",
          authentication: "dedicated_password", savedCredential: "", newCredential: "", keyID: "",
        });
        navigate("Connections");
      },
    },
    {
      id: "open-files",
      label: t("palette.openRemoteFiles"),
      detail: t("palette.openRemoteFilesDetail"),
      search: "sftp files remote browse ファイル リモート 転送",
      run: () => navigate("Files"),
    },
    {
      id: "open-shell",
      label: t("palette.openLocalShell"),
      detail: t("palette.openLocalShellDetail"),
      search: "shell local terminal console シェル ローカル ターミナル",
      run: () => void openLocalShell(),
    },
    ...(!session.passwordless ? [{
      id: "lock-vault",
      label: t("palette.lockVault"),
      detail: t("palette.lockVaultDetail"),
      search: "lock vault secure ロック 保管庫 施錠",
      run: () => {
        void vaultApi.lockVault().then((status) => {
          if (status.unlocked) session.openVault(status);
          else session.lock();
        }).catch(() => undefined);
      },
    }] : []),
  ];

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
      <IconSprite />
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
            onNavigate={(event, name) => {
              setNavigationOpen(false);
              followSectionLink(event, name);
            }}
            consoles={consoles}
            orderedConsoles={orderedConsoles}
            activeConsole={activeConsole}
            liveWorkspace={liveWorkspace}
            onRenameWorkspace={renameWorkspace}
            unreadBySession={unreadSessions}
            onShowConsole={showConsole}
            onDuplicateConsole={(id) => void duplicateConsole(id)}
            onReorderConsoles={setConsoleOrder}
            localShellProfiles={localShellProfiles}
            onOpenShell={(profileId) => void openLocalShell(profileId)}
            aliases={declared.knownAliases}
            hosts={declared.hosts}
            onConnect={(alias) => void (async () => {
              const opened = await consoles.open({ kind: "ssh", alias });
              if (opened !== null) showConsole(opened.id);
            })()}
            onOpenCommandPalette={() => {
              commandPaletteReturnFocusRef.current = navigationTriggerRef.current;
              setNavigationOpen(false);
              setCommandPaletteOpen(true);
            }}
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
            {vaultMigration === null ? null : (
              <div
                role="status"
                className="flex shrink-0 items-center gap-3 border-b border-notice-line bg-notice px-4 py-2 text-sm text-notice-ink"
              >
                <p className="min-w-0 grow">
                  {t("lock.migrationCompleted", {
                    current: vaultMigration.from,
                    required: vaultMigration.to,
                  })}
                </p>
                <Button
                  className="shrink-0"
                  onClick={session.clearVaultMigration}
                >
                  {t("lock.migrationDismiss")}
                </Button>
              </div>
            )}
            {handoffs.connectionDraft !== null &&
            (section === "Groups" || section === "Keys") ? (
              <div className="flex shrink-0 items-center gap-3 border-b border-notice-line bg-notice px-4 py-2 text-sm text-notice-ink">
                <p className="min-w-0 grow truncate">
                  {t("conn.createDraftWaiting", {
                    alias:
                      handoffs.connectionDraft.alias || t("conn.createUntitledDraft"),
                  })}
                </p>
                <Button
                  className="shrink-0"
                  onClick={() => navigate("Connections")}
                >
                  {t("conn.createReturnToDraft")}
                </Button>
              </div>
            ) : null}
            {state === "ready" ? (
              <div className="relative min-h-0 flex-1 overflow-hidden">
                {terminalFace || activeConsole !== null ? (
                  <div className={terminalFace ? "h-full" : "hidden"}>
                    <TerminalScreen
                      visible={terminalFace}
                      consoles={consoles}
                      activeConsole={activeConsole}
                      settings={terminalSettings}
                      hostAppearance={declared.hostAppearance}
                      hostOSC52={declared.hostOSC52}
                      onActive={showConsole}
                      onLiveWorkspaceChange={setLiveWorkspace}
                      onOpenAlias={(alias) =>
                        consoles.open({ kind: "ssh", alias })
                      }
                      onOpenShell={() => consoles.open({ kind: "shell" })}
                      onOSC52Change={changeOSC52}
                      onOpenRemotePath={handoffs.openRemotePath}
                      restoreRequest={workspaceRestoreRequest}
                      onRestoreConsumed={consumeWorkspaceRestore}
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
                        onInspector: setInspector,
                        consoles,
                        onShowConsole: showConsole,
                        onOpenWorkspace: openWorkspace,
                        onTerminalSettingsChange: async (settings) => {
                          setTerminalSettings(settings);
                          await consoles.refresh();
                        },
                      }}
                      declared={{ groups: declared.groups, knownAliases: declared.knownAliases, hosts: declared.hosts }}
                      sftpTarget={handoffs.sftpTarget}
                      onSftpTargetHandled={handoffs.handleSftpTarget}
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
          <nav aria-label={t("shell.mobileNavigation")} className="sshc-mobile-navigation grid shrink-0 grid-cols-5 border-t border-line bg-toolbar">
            {(["Home", "Connections", "Files", "Terminal", "Menu"] as const).map((name) => (
              <a
                key={name}
                href={sectionPath(name)}
                aria-current={section === name ? "page" : undefined}
                onClick={(event) => {
                  setNavigationOpen(false);
                  followSectionLink(event, name);
                }}
                className={`relative flex min-h-13 min-w-0 flex-col items-center justify-center gap-0.5 px-1 py-1 text-[10px] transition-colors ${section === name ? "bg-select-fill font-semibold text-accent" : "text-ink-muted active:bg-hover"}`}
              >
                {section === name ? <span aria-hidden="true" className="absolute inset-x-4 top-0 h-0.5 rounded-full bg-accent" /> : null}
                <Icon name={sectionIcons[name]} className="size-5" />
                <span className="max-w-full truncate">{t(sectionLabels[name])}</span>
              </a>
            ))}
          </nav>
        ) : null}
        {state === "ready" ? <TransferNotifications /> : null}
        {state === "ready" ? (
          <CommandPalette
            open={commandPaletteOpen}
            commands={paletteCommands}
            returnFocusRef={commandPaletteReturnFocusRef}
            hosts={declared.hosts}
            files={declared.files}
            sessions={orderedConsoles}
            unreadBySession={unreadSessions}
            sectionLabels={sectionLabels}
            onClose={() => setCommandPaletteOpen(false)}
            onConnect={async (alias) => {
              const opened = await consoles.open({ kind: "ssh", alias });
              if (opened !== null) showConsole(opened.id);
            }}
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
            onOpenSession={showConsole}
          />
        ) : null}
      </div>
      {state === "ready" && vaultRecheck !== "idle" ? <VaultRecheckOverlay phase={vaultRecheck} /> : null}
    </div>
  );
}

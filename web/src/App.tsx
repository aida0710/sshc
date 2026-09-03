import {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react";
import { type HealthResponse } from "./api/client";
import {
  integrationsApi,
  type PasswordVaultStatus,
  type TerminalAppearance,
  type TerminalSettings,
} from "./api/integrations";
import { resolveAppearance } from "./terminal/appearance";
import { configApi, type FileNode, type HostEntry } from "./api/config";
import type { SessionState } from "./session/bootstrap";
import type {
  CreateConnectionDraft,
  CreationPrerequisite,
} from "./connections/CreateConnectionModal";
import type { FileTarget } from "./explorer/ConfigExplorer";
import { LockScreen } from "./secrets/LockScreen";
import { OverviewPanel } from "./overview/OverviewPanel";
import { useLanguage } from "./i18n/context";
import { secondaryAction } from "./ui/form";
import { IconSprite, type IconName } from "./ui/icons";
import { InspectorPane, InspectorToggle, type InspectorContent } from "./ui/Inspector";
import { useTheme } from "./theme/context";
import type { MessageKey } from "./i18n/messages";
import { Button, Card } from "./ui/surface";
import { RouteSkeleton } from "./ui/RouteSkeleton";
import { sectionPath, type Section } from "./routing/sectionRoute";
import { connectionLocation } from "./routing/connectionRoute";
import { AppHeader } from "./shell/AppHeader";
import { AppNavigation } from "./shell/AppNavigation";
import { MenuPanel, type MenuGroup } from "./shell/MenuPanel";
import {
  clampNavigationWidth,
  detectNavigationWidth,
  rememberNavigationWidth,
} from "./shell/navigationLayout";
import type {
  Declared,
  Handoff,
  Navigation,
  Shell,
} from "./shell/sectionProps";
import { useSectionRoute } from "./routing/useSectionRoute";
import type {
  GeneratedPrivateKeyHandoff,
  GeneratedPublicKeyHandoff,
} from "./keys/workflow";
import {
  useTerminalSessions,
  type TerminalSessionsState,
} from "./terminal/sessions";
import {
  TerminalWorkspace,
  type WorkspaceRenameRequest,
  type WorkspaceRestoreRequest,
} from "./features/workspaces/TerminalWorkspace";
import type { LiveWorkspaceSummary } from "./features/workspaces/live";
import { TransferNotifications } from "./sftp/TransferNotifications";
import { sftpTransferManager } from "./sftp/transferManager";
import { ErrorDiagnosticNotice } from "./shell/ErrorDiagnosticNotice";
import { CommandPalette, type PaletteCommand } from "./shell/CommandPalette";
import { setAndroidAppearance } from "./android/native";
import { useAgentNotifications } from "./terminal/agentNotifications";
import type { RemotePathAction } from "./terminal/TerminalLinkPopover";
import type { SFTPTarget } from "./sftp/SFTPPanel";
import { useAppSession } from "./session/useAppSession";
import { useTerminalWorkspaceController } from "./terminal/useTerminalWorkspaceController";
import { useDismissibleLayer } from "./ui/useDismissibleLayer";
import { useMediaQuery } from "./ui/useMediaQuery";
import {
  parseSettingsPage,
  settingsPageMeta,
  settingsPages,
} from "./settings/settingsRoute";

export { vaultStatePollIntervalMs } from "./session/useAppSession";

const TerminalView = lazy(() =>
  import("./terminal/TerminalView").then(({ TerminalView }) => ({
    default: TerminalView,
  })),
);
const ConnectionsPage = lazy(() =>
  import("./connections/ConnectionsPage").then(({ ConnectionsPage }) => ({
    default: ConnectionsPage,
  })),
);
const ConfigExplorer = lazy(() =>
  import("./explorer/ConfigExplorer").then(({ ConfigExplorer }) => ({
    default: ConfigExplorer,
  })),
);
const GroupsPanel = lazy(() =>
  import("./groups/GroupsPanel").then(({ GroupsPanel }) => ({
    default: GroupsPanel,
  })),
);
const HistoryPanel = lazy(() =>
  import("./history/HistoryPanel").then(({ HistoryPanel }) => ({
    default: HistoryPanel,
  })),
);
const KeysScreen = lazy(() =>
  import("./keys/KeysScreen").then(({ KeysScreen }) => ({
    default: KeysScreen,
  })),
);
const DiagnosticsPanel = lazy(() =>
  import("./diagnostics/DiagnosticsPanel").then(({ DiagnosticsPanel }) => ({
    default: DiagnosticsPanel,
  })),
);
const SecretsPanel = lazy(() =>
  import("./secrets/SecretsPanel").then(({ SecretsPanel }) => ({
    default: SecretsPanel,
  })),
);
const SettingsPanel = lazy(() =>
  import("./settings/SettingsPanel").then(({ SettingsPanel }) => ({
    default: SettingsPanel,
  })),
);
const SyncPanel = lazy(() =>
  import("./sync/SyncPanel").then(({ SyncPanel }) => ({ default: SyncPanel })),
);
const KnownHostsPanel = lazy(() =>
  import("./knownhosts/KnownHostsPanel").then(({ KnownHostsPanel }) => ({
    default: KnownHostsPanel,
  })),
);
const RemoteKeyPanel = lazy(() =>
  import("./remotekeys/RemoteKeyPanel").then(({ RemoteKeyPanel }) => ({
    default: RemoteKeyPanel,
  })),
);
const SFTPWorkspace = lazy(() =>
  import("./sftp/SFTPWorkspace").then(({ SFTPWorkspace }) => ({
    default: SFTPWorkspace,
  })),
);
const SnippetsPanel = lazy(() =>
  import("./snippets/SnippetsPanel").then(({ SnippetsPanel }) => ({
    default: SnippetsPanel,
  })),
);

type AppProps = {
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  vault?: () => Promise<PasswordVaultStatus>;
};

const sectionLabels: Record<Section, MessageKey> = {
  Home: "section.home",
  Menu: "section.menu",
  Connections: "section.connections",
  Terminal: "section.terminal",
  Files: "section.files",
  Snippets: "section.snippets",
  Config: "section.config",
  Groups: "section.groups",
  Keys: "section.keys",
  "Known Hosts": "section.knownHosts",
  "Remote Keys": "section.remoteKeys",
  Diagnostics: "section.diagnostics",
  Secrets: "section.secrets",
  Settings: "section.settings",
  Sync: "section.sync",
  History: "section.history",
};

const sectionIcons: Record<Section, IconName> = {
  Home: "home",
  Menu: "menu",
  Connections: "connections",
  Terminal: "terminal",
  Files: "config",
  Snippets: "terminal",
  Config: "config",
  Groups: "groups",
  Keys: "keys",
  "Known Hosts": "knownHosts",
  "Remote Keys": "remoteKeys",
  Diagnostics: "diagnostics",
  Secrets: "secrets",
  Settings: "settings",
  Sync: "sync",
  History: "history",
};

const startSections: Section[] = ["Home", "Connections", "Files"];

export function resolveOSC52(
  policy: "allow" | "deny" | undefined,
  fallback: boolean,
): boolean {
  return policy === undefined ? fallback : policy === "allow";
}

const navGroups: { label: MessageKey; sections: Section[] }[] = [
  { label: "shell.navStart", sections: startSections },
  { label: "shell.navConnections", sections: ["Config", "Groups"] },
  {
    label: "shell.navKeysHosts",
    sections: ["Keys", "Known Hosts", "Remote Keys"],
  },
  {
    label: "shell.navMaintenance",
    sections: [
      "Diagnostics",
      "Secrets",
      "Snippets",
      "Settings",
      "Sync",
      "History",
    ],
  },
];

function sectionMenuItems(sections: Section[]) {
  return sections.map((section) => ({
    key: section,
    label: sectionLabels[section],
    icon: sectionIcons[section],
    href: sectionPath(section),
  }));
}

const menuGroups: MenuGroup[] = [
  ...navGroups.slice(1, 3).map((group) => ({
    label: group.label,
    items: sectionMenuItems(group.sections),
  })),
  {
    label: "section.settings",
    items: settingsPages.map((page) => ({
      key: page,
      label: settingsPageMeta[page].label,
      icon: settingsPageMeta[page].icon,
      href: settingsPageMeta[page].path,
    })),
  },
  {
    label: "shell.navMaintenance",
    items: sectionMenuItems([
      "Diagnostics",
      "Secrets",
      "Snippets",
      "Sync",
      "History",
    ]),
  },
];

const navigationId = "primary-navigation";
export function App({
  bootstrap,
  health,
  vault = integrationsApi.passwordVault,
}: AppProps) {
  const { t } = useLanguage();
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
  const [fileTarget, setFileTarget] = useState<FileTarget | null>(null);
  const [sftpTarget, setSftpTarget] = useState<SFTPTarget | null>(null);
  const sftpTargetSequence = useRef(0);
  const [groups, setGroups] = useState<string[]>([]);
  const [hostAppearance, setHostAppearance] = useState<
    Map<string, TerminalAppearance>
  >(new Map());
  const [hostOSC52, setHostOSC52] = useState<Map<string, "allow" | "deny">>(
    new Map(),
  );
  const [knownAliases, setKnownAliases] = useState<string[]>([]);
  const [paletteHosts, setPaletteHosts] = useState<HostEntry[]>([]);
  const [configFiles, setConfigFiles] = useState<FileNode[]>([]);
  const [connectionDraft, setConnectionDraft] =
    useState<CreateConnectionDraft | null>(null);
  const [preferredConnectionKey, setPreferredConnectionKey] =
    useState<GeneratedPrivateKeyHandoff | null>(null);
  const [preferredPublicKey, setPreferredPublicKey] =
    useState<GeneratedPublicKeyHandoff | null>(null);
  const consumePreferredConnectionKey = useCallback(
    () => setPreferredConnectionKey(null),
    [],
  );
  const consumePreferredPublicKey = useCallback(
    () => setPreferredPublicKey(null),
    [],
  );
  const [navigationOpen, setNavigationOpen] = useState(false);
  const navigationPanelRef = useRef<HTMLElement>(null);
  const navigationTriggerRef = useRef<HTMLButtonElement>(null);
  const [desktopNavigationWidth, setDesktopNavigationWidth] = useState(
    detectNavigationWidth,
  );
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
    if (state !== "ready") {
      setCommandPaletteOpen(false);
      return;
    }
    function togglePalette(event: KeyboardEvent) {
      if (
        !(event.ctrlKey || event.metaKey) ||
        event.altKey ||
        event.key.toLocaleLowerCase() !== "k"
      )
        return;
      event.preventDefault();
      commandPaletteReturnFocusRef.current =
        document.activeElement instanceof HTMLElement ? document.activeElement : null;
      setCommandPaletteOpen((open) => !open);
    }
    document.addEventListener("keydown", togglePalette);
    return () => document.removeEventListener("keydown", togglePalette);
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

  function resizeDesktopNavigation(width: number) {
    const nextWidth = clampNavigationWidth(width);
    setDesktopNavigationWidth(nextWidth);
    rememberNavigationWidth(nextWidth);
  }
  const consoles = useTerminalSessions(integrationsApi, t, state === "ready");
  const closeNavigation = useCallback(() => setNavigationOpen(false), []);
  const terminalWorkspace = useTerminalWorkspaceController({
    api: integrationsApi,
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

  useEffect(() => {
    if (state !== "ready") return;
    const refresh = () => {
      void sftpTransferManager.reconcile().catch(() => undefined);
    };
    refresh();
    const timer = globalThis.setInterval(refresh, 2_000);
    return () => globalThis.clearInterval(timer);
  }, [state]);

  const unreadAgentSessions = useAgentNotifications(
    consoles.sessions,
    terminalFace ? activeConsole : null,
    t,
    showConsole,
  );

  useEffect(() => {
    setInspector(null);
  }, [section]);

  function openFile(path: string, line: number) {
    setFileTarget({ path, line });
    navigate("Config");
  }

  function openRemotePath(
    alias: string,
    path: string,
    action: RemotePathAction,
  ) {
    sftpTargetSequence.current += 1;
    setSftpTarget({ alias, path, action, request: sftpTargetSequence.current });
    navigate("Files");
  }

  // Ctrl/Cmd+K performs as well as navigates. These are the actions that are
  // otherwise several clicks deep from wherever the user happens to be.
  const paletteCommands: PaletteCommand[] = [
    {
      id: "new-connection",
      label: t("palette.newConnection"),
      detail: t("palette.newConnectionDetail"),
      search: "new connection create host add 新規 接続 追加 作成",
      run: () => {
        setConnectionDraft({
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
    {
      id: "lock-vault",
      label: t("palette.lockVault"),
      detail: t("palette.lockVaultDetail"),
      search: "lock vault secure ロック 保管庫 施錠",
      run: () => session.lock(),
    },
  ];

  function assignGeneratedKey(key: GeneratedPrivateKeyHandoff) {
    setPreferredConnectionKey(key);
    navigate("Connections");
  }

  function installGeneratedKey(key: GeneratedPublicKeyHandoff) {
    setPreferredPublicKey(key);
    navigate("Remote Keys");
  }

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

  useEffect(() => {
    if (state !== "ready") return;
    let active = true;
    void configApi
      .overview()
      .then((overview) => {
        if (!active) return;
        setGroups((overview.metadata.groups ?? []).map((group) => group.name));
        setHostAppearance(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.appearance === undefined || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.appearance] as const],
            ),
          ),
        );
        setHostOSC52(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.osc52 === undefined || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.osc52] as const],
            ),
          ),
        );
        setKnownAliases([
          ...new Set(
            overview.hosts
              .map((host) => host.identity.alias)
              .filter((alias) => alias !== ""),
          ),
        ]);
        setPaletteHosts(
          overview.hosts.filter((host) => host.identity.alias !== ""),
        );
        setConfigFiles(overview.files);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [state, section]);

  if (state === "locked") {
    return (
      <LockScreen
        exists={vaultExists}
        version={version}
        onExists={session.markVaultExists}
        onOpen={session.openVault}
      />
    );
  }

  if (state === "error") {
    return (
      <div className="min-h-screen bg-canvas text-ink">
        {requestFailure === null ? null : (
          <ErrorDiagnosticNotice
            diagnostic={requestFailure}
            version={version}
            onClose={session.clearRequestFailure}
          />
        )}
        <main className="flex flex-col items-start gap-3 p-6">
          <h1 className="text-base font-semibold">{t("shell.title")}</h1>
          <p role="alert" className="text-sm text-danger">
            {t("shell.bootstrapFailed")}
          </p>

          {failure === "" ? null : (
            <code className="max-w-full overflow-x-auto rounded-md border border-line bg-card px-2 py-1 text-xs">
              {failure}
            </code>
          )}

          <Button
            kind="primary"
            onClick={() =>
              window.location.replace(
                window.location.pathname + window.location.search,
              )
            }
          >
            {t("shell.bootstrapRetry")}
          </Button>
        </main>
      </div>
    );
  }

  if (state === "session-ended") {
    return (
      <main className="grid min-h-screen place-items-center bg-canvas p-6 text-ink">
        <Card as="section" className="flex w-full max-w-md flex-col items-start gap-4 p-6 sm:p-8">
          <h1 className="text-lg font-semibold">
            {t("shell.sessionEndedHeading")}
          </h1>
          <p role="alert" className="text-sm leading-6 text-ink-muted">
            {t("shell.sessionEnded")}
          </p>
          <Button
            kind="primary"
            onClick={() =>
              window.location.replace(
                window.location.pathname + window.location.search,
              )
            }
          >
            {t("shell.sessionReload")}
          </Button>
        </Card>
      </main>
    );
  }

  return (
    <div className="flex h-screen flex-col bg-canvas text-ink">
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
          className={`grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[var(--navigation-width)_minmax(0,1fr)] ${
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
              className="fixed inset-0 z-20 bg-canvas/70 md:hidden"
            />
          ) : null}
          <AppNavigation
            navigationRef={navigationPanelRef}
            navigationId={navigationId}
            version={version}
            state={state}
            navigationOpen={navigationOpen}
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
            unreadBySession={unreadAgentSessions}
            onShowConsole={showConsole}
            onDuplicateConsole={(id) => void duplicateConsole(id)}
            onReorderConsoles={setConsoleOrder}
            localShellProfiles={localShellProfiles}
            onOpenShell={(profileId) => void openLocalShell(profileId)}
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
            {connectionDraft !== null &&
            (section === "Groups" || section === "Keys") ? (
              <div className="flex shrink-0 items-center gap-3 border-b border-notice-line bg-notice px-4 py-2 text-sm text-notice-ink">
                <p className="min-w-0 grow truncate">
                  {t("conn.createDraftWaiting", {
                    alias:
                      connectionDraft.alias || t("conn.createUntitledDraft"),
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
                      consoles={consoles}
                      activeConsole={activeConsole}
                      settings={terminalSettings}
                      hostAppearance={hostAppearance}
                      hostOSC52={hostOSC52}
                      onActive={showConsole}
                      onLiveWorkspaceChange={setLiveWorkspace}
                      onOpenAlias={(alias) =>
                        consoles.open({ kind: "ssh", alias })
                      }
                      onOpenShell={() => consoles.open({ kind: "shell" })}
                      onOSC52Change={async (session, enabled) => {
                        if (
                          session.kind !== "ssh" ||
                          session.alias === undefined
                        ) {
                          const next = { ...terminalSettings };
                          if (enabled) next.osc52 = true;
                          else delete next.osc52;
                          await integrationsApi.setTerminalSettings(next);
                          setTerminalSettings(next);
                          return;
                        }
                        const overview = await configApi.overview();
                        const identity = overview.hosts.find(
                          (host) => host.identity.alias === session.alias,
                        )?.identity;
                        if (identity === undefined)
                          throw new Error("host_not_found");
                        const hosts = [...(overview.metadata.hosts ?? [])];
                        const index = hosts.findIndex(
                          (host) =>
                            host.identity.path === identity.path &&
                            host.identity.alias === identity.alias,
                        );
                        const updated = {
                          ...(index < 0 ? {} : hosts[index]),
                          identity,
                          osc52: enabled
                            ? ("allow" as const)
                            : ("deny" as const),
                        };
                        if (index < 0) hosts.push(updated);
                        else hosts[index] = updated;
                        await configApi.save({
                          kind: "metadata",
                          metadata: { ...overview.metadata, hosts },
                        });
                        setHostOSC52((current) =>
                          new Map(current).set(session.alias!, updated.osc52),
                        );
                      }}
                      onOpenRemotePath={openRemotePath}
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
                        fileTarget,
                        onNavigate: navigate,
                        onNavigateLocation: navigateLocation,
                        onNavigateForCreation: (target: CreationPrerequisite) =>
                          navigate(target),
                        onOpenFile: openFile,
                        onNavigationBlockerChange: setNavigationBlocker,
                      }}
                      handoff={{
                        connectionKey: preferredConnectionKey,
                        publicKey: preferredPublicKey,
                        connectionDraft,
                        onAssignGeneratedKey: assignGeneratedKey,
                        onInstallGeneratedKey: installGeneratedKey,
                        onConnectionKeyApplied: consumePreferredConnectionKey,
                        onPublicKeyHandled: consumePreferredPublicKey,
                        onConnectionDraftChange: setConnectionDraft,
                      }}
                      shell={{
                        onLock: session.lock,
                        onInspector: setInspector,
                        consoles,
                        onShowConsole: showConsole,
                        onOpenWorkspace: openWorkspace,
                        onTerminalSettingsChange: async (settings) => {
                          setTerminalSettings(settings);
                          await consoles.refresh();
                        },
                      }}
                      declared={{ groups, knownAliases, hosts: paletteHosts }}
                      sftpTarget={sftpTarget}
                      onSftpTargetHandled={(request) =>
                        setSftpTarget((current) =>
                          current?.request === request ? null : current,
                        )
                      }
                    />
                  </Suspense>
                ) : (
                  <div className="h-full overflow-y-auto p-4 md:p-5">
                    <section
                      aria-labelledby="not-found-heading"
                      className="flex max-w-2xl flex-col gap-3"
                    >
                      <h2 id="not-found-heading" className="font-medium">
                        {t("shell.pageNotFound")}
                      </h2>
                      <p className="text-sm text-ink-muted">
                        {t("shell.pageNotFoundDescription")}
                      </p>
                      <code className="w-fit rounded-md border border-line bg-card px-2 py-1 text-sm">
                        {route.pathname}
                      </code>
                      <a
                        href={sectionPath("Home")}
                        onClick={(event) => followSectionLink(event, "Home")}
                        className={`${secondaryAction} w-fit`}
                      >
                        {t("shell.goHome")}
                      </a>
                    </section>
                  </div>
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
        {state === "ready" ? <TransferNotifications /> : null}
        {state === "ready" ? (
          <CommandPalette
            open={commandPaletteOpen}
            commands={paletteCommands}
            returnFocusRef={commandPaletteReturnFocusRef}
            hosts={paletteHosts}
            files={configFiles}
            sessions={orderedConsoles}
            unreadBySession={unreadAgentSessions}
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
            onOpenFile={(path) => openFile(path, 1)}
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
      {state === "ready" && vaultRecheck !== "idle" ? (
        <div className="fixed inset-0 z-[100] grid min-h-screen place-items-center bg-canvas p-6">
          <Card
            as="section"
            role="status"
            className="w-full max-w-md p-6 sm:p-8"
          >
            <h1 className="text-lg font-semibold">{t("shell.title")}</h1>
            <p className="mt-3 text-sm leading-6 text-ink-muted">
              {t(
                vaultRecheck === "checking"
                  ? "shell.vaultChecking"
                  : "shell.vaultCheckRetrying",
              )}
            </p>
          </Card>
        </div>
      ) : null}
    </div>
  );
}

type SectionViewProps = {
  section: Section;
  navigation: Navigation;
  handoff: Handoff;
  shell: Shell;
  declared: Declared;
  sftpTarget: SFTPTarget | null;
  onSftpTargetHandled: (request: number) => void;
};

function SectionView(props: SectionViewProps) {
  if (props.section === "Terminal") {
    return null;
  }
  if (props.section === "Connections") {
    return (
      <ConnectionsPage
        onInspector={props.shell.onInspector}
        creationDraft={props.handoff.connectionDraft}
        onCreationDraftChange={props.handoff.onConnectionDraftChange}
        onNavigateForCreation={props.navigation.onNavigateForCreation}
        location={props.navigation.location}
        onNavigateLocation={props.navigation.onNavigateLocation}
        onNavigationBlockerChange={props.navigation.onNavigationBlockerChange}
        preferredKey={props.handoff.connectionKey}
        onPreferredKeyApplied={props.handoff.onConnectionKeyApplied}
        consoles={props.shell.consoles}
        onShowConsole={props.shell.onShowConsole}
      />
    );
  }
  return (
    <div className={props.section === "Files"
      ? "h-full overflow-y-auto px-4 pb-4 md:px-5 md:pb-5"
      : "h-full overflow-y-auto p-4 md:p-5"}
    >
      {<PaddedSection {...props} />}
    </div>
  );
}

function TerminalScreen({
  consoles,
  activeConsole,
  settings,
  hostAppearance,
  hostOSC52,
  onActive,
  onLiveWorkspaceChange,
  onOpenAlias,
  onOpenShell,
  restoreRequest,
  onRestoreConsumed,
  renameRequest,
  onRenameConsumed,
  onOpenRemotePath,
  onOSC52Change,
}: {
  consoles: TerminalSessionsState;
  activeConsole: string | null;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
  hostOSC52: Map<string, "allow" | "deny">;
  onActive: (id: string) => void;
  onLiveWorkspaceChange: (workspace: LiveWorkspaceSummary | null) => void;
  onOpenAlias: (
    alias: string,
  ) => Promise<import("./api/integrations").TerminalSession | null>;
  onOpenShell: () => Promise<
    import("./api/integrations").TerminalSession | null
  >;
  restoreRequest: WorkspaceRestoreRequest | null;
  onRestoreConsumed: (sequence: number) => void;
  renameRequest: WorkspaceRenameRequest | null;
  onRenameConsumed: (sequence: number) => void;
  onOpenRemotePath: (
    alias: string,
    path: string,
    action: RemotePathAction,
  ) => void;
  onOSC52Change: (
    session: import("./api/integrations").TerminalSession,
    enabled: boolean,
  ) => Promise<void>;
}) {
  return (
    <TerminalWorkspace
      sessions={consoles.sessions}
      sessionsLoaded={consoles.loaded}
      activeSessionId={activeConsole}
      onActive={onActive}
      onOpenAlias={onOpenAlias}
      onOpenShell={onOpenShell}
      restoreRequest={restoreRequest}
      onRestoreConsumed={onRestoreConsumed}
      renameRequest={renameRequest}
      onRenameConsumed={onRenameConsumed}
      onLiveWorkspaceChange={onLiveWorkspaceChange}
      renderTerminal={(session) => {
        const appearance = resolveAppearance(
          session.alias === undefined
            ? undefined
            : hostAppearance.get(session.alias),
          settings.appearance,
        );
        const hostPolicy =
          session.alias === undefined
            ? undefined
            : hostOSC52.get(session.alias);
        const osc52Enabled = resolveOSC52(hostPolicy, settings.osc52 ?? false);
        return (
          <Suspense fallback={<RouteSkeleton kind="terminal" />}>
            <TerminalView
              key={session.id}
              session={session}
              {...(settings.fontSize === undefined
                ? {}
                : { fontSize: settings.fontSize })}
              {...(settings.browserScrollbackLines === undefined
                ? {}
                : { scrollbackLines: settings.browserScrollbackLines })}
              osc52Enabled={osc52Enabled}
              jisYenBackslash={settings.jisYenBackslash ?? false}
              onOsc52Change={(enabled) => onOSC52Change(session, enabled)}
              onForwardsChanged={consoles.refresh}
              {...(appearance.palette === ""
                ? {}
                : { palette: appearance.palette })}
              {...(appearance.font === "" ? {} : { font: appearance.font })}
              {...(appearance.background === ""
                ? {}
                : { background: appearance.background })}
              {...(appearance.tint === undefined
                ? {}
                : { tint: appearance.tint })}
              copyOnSelect={settings.copyOnSelect ?? true}
              rightClickPaste={settings.rightClickPaste ?? true}
              webgl={settings.webgl ?? true}
              onExit={() => consoles.markExited(session.id)}
              onReconnect={() => consoles.reconnect(session.id)}
              onResumeAgent={async (placement) => {
                const resumed =
                  (await consoles.resumeAgent?.(
                    session.id,
                    session.agent?.observationVersion ?? 0,
                    placement,
                  )) ?? null;
                if (resumed === null) return false;
                if (placement === "new-pane") onActive(resumed.id);
                return true;
              }}
              onOpenRemotePath={onOpenRemotePath}
            />
          </Suspense>
        );
      }}
    />
  );
}

function PaddedSection({
  section,
  navigation,
  handoff,
  shell,
  declared,
  sftpTarget,
  onSftpTargetHandled,
}: SectionViewProps) {
  const { fileTarget, onNavigate, onNavigateLocation } = navigation;
  const {
    onLock,
    onInspector,
    consoles,
    onShowConsole,
    onOpenWorkspace,
    onTerminalSettingsChange,
  } = shell;
  if (section === "Home") {
    return (
      <OverviewPanel
        onNavigate={onNavigate}
        onNavigateLocation={onNavigateLocation}
        onConsoleOpened={onShowConsole}
        onOpenWorkspace={onOpenWorkspace}
      />
    );
  }
  if (section === "Menu") {
    return (
      <MenuPanel
        groups={menuGroups}
        onNavigate={onNavigateLocation}
      />
    );
  }
  if (section === "Config") {
    return <ConfigExplorer target={fileTarget} />;
  }
  if (section === "Files") {
    return (
      <SFTPWorkspace
        aliases={declared.knownAliases}
        hosts={declared.hosts}
        target={sftpTarget}
        onTargetHandled={onSftpTargetHandled}
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
        onOpenTerminal={async (alias, path) => {
          const opened = await consoles.open({ kind: "ssh", alias, cwd: path });
          if (opened !== null) onShowConsole(opened.id);
        }}
      />
    );
  }
  if (section === "Snippets") {
    return (
      <SnippetsPanel
        aliases={declared.knownAliases}
        selectedSnippetId={new URLSearchParams(navigation.location.search).get(
          "snippet",
        )}
      />
    );
  }
  if (section === "Groups") {
    return <GroupsPanel onInspector={onInspector} />;
  }
  if (section === "Secrets") {
    return <SecretsPanel onLock={onLock} />;
  }
  if (section === "Settings") {
    return (
      <SettingsPanel
        page={parseSettingsPage(navigation.location.pathname) ?? "Engine"}
        consoles={consoles}
        onTerminalSettingsChange={onTerminalSettingsChange}
      />
    );
  }
  if (section === "Sync") {
    return <SyncPanel />;
  }
  if (section === "History") {
    return <HistoryPanel />;
  }
  if (section === "Keys") {
    return (
      <KeysScreen
        onInspector={onInspector}
        groups={declared.groups}
        onAssignGeneratedKey={handoff.onAssignGeneratedKey}
        onInstallGeneratedKey={handoff.onInstallGeneratedKey}
      />
    );
  }
  if (section === "Known Hosts") {
    return <KnownHostsPanel />;
  }
  if (section === "Remote Keys") {
    return (
      <RemoteKeyPanel
        hosts={declared.knownAliases}
        preferredPublicKeyPath={handoff.publicKey?.publicRelativePath ?? null}
        onPreferredPublicKeyHandled={handoff.onPublicKeyHandled}
      />
    );
  }
  if (section === "Diagnostics") {
    return <DiagnosticsPanel hosts={declared.knownAliases} />;
  }
  return (
    <section aria-labelledby="section-heading" className="flex flex-col gap-4">
      <h2 id="section-heading" className="font-medium">
        {section}
      </h2>
    </section>
  );
}

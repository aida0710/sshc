import {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react";
import { apiClient, whenLocked, type HealthResponse } from "./api/client";
import { integrationsApi, type PasswordVaultStatus, type TerminalAppearance, type TerminalSettings } from "./api/integrations";
import { resolveAppearance } from "./terminal/appearance";
import { configApi } from "./api/config";
import type { SessionState } from "./session/bootstrap";
import type { CreateConnectionDraft, CreationPrerequisite } from "./connections/CreateConnectionModal";
import type { FileTarget } from "./explorer/ConfigExplorer";
import { LockScreen } from "./secrets/LockScreen";
import { OverviewPanel } from "./overview/OverviewPanel";
import { useLanguage, useTranslate } from "./i18n/context";
import type { Locale } from "./i18n/locale";
import { secondaryAction } from "./ui/form";
import { IconSprite, type IconName } from "./ui/icons";
import { InspectorPane, type InspectorContent } from "./ui/Inspector";
import { useTheme } from "./theme/context";
import type { Theme } from "./theme/theme";
import type { MessageKey } from "./i18n/messages";
import { Button } from "./ui/surface";
import { RouteSkeleton } from "./ui/RouteSkeleton";
import { sectionPath, type Section } from "./routing/sectionRoute";
import { AppHeader } from "./shell/AppHeader";
import { AppNavigation, type NavFace } from "./shell/AppNavigation";
import {
  clampNavigationWidth,
  detectNavigationVisible,
  detectNavigationWidth,
  rememberNavigationVisible,
  rememberNavigationWidth,
} from "./shell/navigationLayout";
import type { Declared, Handoff, Navigation, Shell } from "./shell/sectionProps";
import {
  useSectionRoute,
} from "./routing/useSectionRoute";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "./keys/workflow";
import { useTerminalSessions, type TerminalSessionsState } from "./terminal/sessions";

const TerminalView = lazy(() =>
  import("./terminal/TerminalView").then(({ TerminalView }) => ({ default: TerminalView })),
);
const ConnectionsPage = lazy(() =>
  import("./connections/ConnectionsPage").then(({ ConnectionsPage }) => ({ default: ConnectionsPage })),
);
const ConfigExplorer = lazy(() =>
  import("./explorer/ConfigExplorer").then(({ ConfigExplorer }) => ({ default: ConfigExplorer })),
);
const GroupsPanel = lazy(() =>
  import("./groups/GroupsPanel").then(({ GroupsPanel }) => ({ default: GroupsPanel })),
);
const HistoryPanel = lazy(() =>
  import("./history/HistoryPanel").then(({ HistoryPanel }) => ({ default: HistoryPanel })),
);
const KeysScreen = lazy(() =>
  import("./keys/KeysScreen").then(({ KeysScreen }) => ({ default: KeysScreen })),
);
const DiagnosticsPanel = lazy(() =>
  import("./diagnostics/DiagnosticsPanel").then(({ DiagnosticsPanel }) => ({ default: DiagnosticsPanel })),
);
const SecretsPanel = lazy(() =>
  import("./secrets/SecretsPanel").then(({ SecretsPanel }) => ({ default: SecretsPanel })),
);
const SettingsPanel = lazy(() =>
  import("./settings/SettingsPanel").then(({ SettingsPanel }) => ({ default: SettingsPanel })),
);
const SyncPanel = lazy(() =>
  import("./sync/SyncPanel").then(({ SyncPanel }) => ({ default: SyncPanel })),
);
const KnownHostsPanel = lazy(() =>
  import("./knownhosts/KnownHostsPanel").then(({ KnownHostsPanel }) => ({ default: KnownHostsPanel })),
);
const RemoteKeyPanel = lazy(() =>
  import("./remotekeys/RemoteKeyPanel").then(({ RemoteKeyPanel }) => ({ default: RemoteKeyPanel })),
);

type AppProps = {
  bootstrap: () => Promise<SessionState>;
  health: () => Promise<HealthResponse>;
  vault?: () => Promise<PasswordVaultStatus>;
};

const sectionLabels: Record<Section, MessageKey> = {
  Home: "section.home",
  Connections: "section.connections",
  Terminal: "section.terminal",
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

const localeLabels: Record<Locale, MessageKey> = {
  en: "shell.languageEnglish",
  ja: "shell.languageJapanese",
};

const sectionIcons: Record<Section, IconName> = {
  Home: "home",
  Connections: "connections",
  Terminal: "terminal",
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

const navGroups: { label: MessageKey; sections: Section[] }[] = [
  { label: "shell.navStart", sections: ["Home", "Connections", "Terminal"] },
  { label: "shell.navConnections", sections: ["Config", "Groups"] },
  { label: "shell.navKeysHosts", sections: ["Keys", "Known Hosts", "Remote Keys"] },
  { label: "shell.navMaintenance", sections: ["Diagnostics", "Secrets", "Settings", "Sync", "History"] },
];


const themeLabels: Record<Theme, MessageKey> = {
  system: "shell.themeSystem",
  light: "shell.themeLight",
  dark: "shell.themeDark",
};

const navigationId = "primary-navigation";

export function App({ bootstrap, health, vault = integrationsApi.passwordVault }: AppProps) {
  const { t } = useLanguage();
  const { theme, setTheme } = useTheme();
  const { route, location, navigate, navigateLocation, setNavigationBlocker } = useSectionRoute();
  const section = route.kind === "section" ? route.section : null;
  const terminalFace = section === "Terminal";
  const [state, setState] = useState<"starting" | "locked" | "ready" | "error">("starting");
  const [failure, setFailure] = useState("");
  const [vaultExists, setVaultExists] = useState(false);
  const [version, setVersion] = useState("");
  const [fileTarget, setFileTarget] = useState<FileTarget | null>(null);
  const [groups, setGroups] = useState<string[]>([]);
  const [hostAppearance, setHostAppearance] = useState<Map<string, TerminalAppearance>>(new Map());
  const [knownAliases, setKnownAliases] = useState<string[]>([]);
  const [connectionDraft, setConnectionDraft] = useState<CreateConnectionDraft | null>(null);
  const [preferredConnectionKey, setPreferredConnectionKey] = useState<GeneratedPrivateKeyHandoff | null>(null);
  const [preferredPublicKey, setPreferredPublicKey] = useState<GeneratedPublicKeyHandoff | null>(null);
  const consumePreferredConnectionKey = useCallback(() => setPreferredConnectionKey(null), []);
  const consumePreferredPublicKey = useCallback(() => setPreferredPublicKey(null), []);
  const [navigationOpen, setNavigationOpen] = useState(false);
  const [desktopNavigationVisible, setDesktopNavigationVisible] = useState(detectNavigationVisible);
  const [desktopNavigationWidth, setDesktopNavigationWidth] = useState(detectNavigationWidth);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspector, setInspector] = useState<InspectorContent>(null);

  useEffect(() => {
    if (!navigationOpen) return;
    function close(event: KeyboardEvent) {
      if (event.key === "Escape") setNavigationOpen(false);
    }
    document.addEventListener("keydown", close);
    return () => document.removeEventListener("keydown", close);
  }, [navigationOpen]);

  function toggleDesktopNavigation() {
    setDesktopNavigationVisible((visible) => {
      rememberNavigationVisible(!visible);
      return !visible;
    });
  }

  function resizeDesktopNavigation(width: number) {
    const nextWidth = clampNavigationWidth(width);
    setDesktopNavigationWidth(nextWidth);
    rememberNavigationWidth(nextWidth);
  }
  const consoles = useTerminalSessions(integrationsApi, t, state === "ready");
  const [terminalSettings, setTerminalSettings] = useState<TerminalSettings>({});
  const [activeConsole, setActiveConsole] = useState<string | null>(null);
  const [navFace, setNavFace] = useState<NavFace | null>(null);

  useEffect(() => {
    if (state !== "ready") return;
    let active = true;
    void integrationsApi.terminalSettings()
      .then((settings) => {
        if (active) setTerminalSettings(settings);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [state, section]);

  useEffect(() => {
    if (navFace !== null || !consoles.loaded) return;
    setNavFace(consoles.sessions.length > 0 ? "terminal" : "settings");
  }, [consoles.loaded, consoles.sessions.length, navFace]);

  useEffect(() => {
    if (activeConsole === null) return;
    if (!consoles.sessions.some((session) => session.id === activeConsole)) setActiveConsole(null);
  }, [consoles.sessions, activeConsole]);

  useEffect(() => {
    if (activeConsole !== null || consoles.sessions.length === 0) return;
    setActiveConsole(consoles.sessions[0]?.id ?? null);
  }, [consoles.sessions, activeConsole]);

  const showConsole = useCallback(
    (id: string) => {
      setActiveConsole(id);
      setNavigationOpen(false);
      navigate("Terminal");
      void consoles.refresh();
    },
    [navigate, consoles],
  );

  const [consoleOrder, setConsoleOrder] = useState<string[]>([]);
  const orderedConsoles = useMemo(() => {
    const rank = new Map(consoleOrder.map((id, index) => [id, index]));
    return consoles.sessions
      .map((session, index) => ({ session, rank: rank.get(session.id) ?? consoleOrder.length + index }))
      .sort((left, right) => left.rank - right.rank)
      .map((entry) => entry.session);
  }, [consoles.sessions, consoleOrder]);

  const openLocalShell = useCallback(async () => {
    const opened = await consoles.open({ kind: "shell" });
    if (opened !== null) showConsole(opened.id);
  }, [consoles, showConsole]);

  const duplicateConsole = useCallback(
    async (id: string) => {
      const session = consoles.sessions.find((candidate) => candidate.id === id);
      if (session === undefined) return;
      const opened = await consoles.open(
        session.kind === "ssh" && session.alias !== undefined
          ? { kind: "ssh", alias: session.alias }
          : { kind: "shell" },
      );
      if (opened !== null) showConsole(opened.id);
    },
    [consoles, showConsole],
  );

  useEffect(() => {
    setInspector(null);
  }, [section]);

  function openFile(path: string, line: number) {
    setFileTarget({ path, line });
    navigate("Config");
  }

  function assignGeneratedKey(key: GeneratedPrivateKeyHandoff) {
    setPreferredConnectionKey(key);
    navigate("Connections");
  }

  function installGeneratedKey(key: GeneratedPublicKeyHandoff) {
    setPreferredPublicKey(key);
    navigate("Remote Keys");
  }

  function followSectionLink(event: MouseEvent<HTMLAnchorElement>, target: Section) {
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

  const currentFace: NavFace = navFace ?? "settings";


  useEffect(() => {
    let active = true;
    void bootstrap()
      .then((sessionState) => {
        if (!active) return null;
        apiClient.setCSRF(sessionState.csrfToken);
        return health();
      })
      .then((result) => {
        if (!active || result === null) return null;
        setVersion(result.version);
        return vault();
      })
      .then((status) => {
        if (!active || status === null) return;
        setVaultExists(status.exists);
        if (!status.unlocked) {
          setState("locked");
          return;
        }
        setState("ready");
      })
      .catch((reason: unknown) => {
        console.error("sshc could not start its session", reason);
        if (!active) return;
        setFailure(reason instanceof Error ? reason.message : String(reason));
        setState("error");
      });

    return () => {
      active = false;
      apiClient.clear();
    };
  }, [bootstrap, health, vault]);

  useEffect(() => {
    whenLocked(() => {
      setVaultExists(true);
      setState("locked");
    });
    return () => whenLocked(null);
  }, []);

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
        setKnownAliases([...new Set(overview.hosts.map((host) => host.identity.alias).filter((alias) => alias !== ""))]);
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
        onOpen={() => {
          setVaultExists(true);
          setState("ready");
        }}
      />
    );
  }

  if (state === "error") {
    return (
      <main className="flex flex-col items-start gap-3 p-6">
        <h1 className="text-base font-semibold">{t("shell.title")}</h1>
        <p role="alert" className="text-sm text-danger">{t("shell.bootstrapFailed")}</p>

        {failure === "" ? null : (
          <code className="max-w-full overflow-x-auto rounded-md border border-line bg-card px-2 py-1 text-xs">
            {failure}
          </code>
        )}

        <Button
          kind="primary"
          onClick={() => window.location.replace(window.location.pathname + window.location.search)}
        >
          {t("shell.bootstrapRetry")}
        </Button>
      </main>
    );
  }

  return (
    <div className="flex h-screen flex-col bg-canvas text-ink">
      <IconSprite />

      <AppHeader
        route={route}
        version={version}
        state={state}
        navigationOpen={navigationOpen}
        desktopNavigationVisible={desktopNavigationVisible}
        navigationId={navigationId}
        onToggleNavigation={() => setNavigationOpen((open) => !open)}
        onToggleDesktopNavigation={toggleDesktopNavigation}
        inspector={inspector}
        inspectorOpen={inspectorOpen}
        onToggleInspector={() => setInspectorOpen((open) => !open)}
        sectionLabels={sectionLabels}
        themeLabels={themeLabels}
        localeLabels={localeLabels}
        theme={theme}
        onThemeChange={setTheme}
      />
      <div
        data-desktop-navigation-visible={desktopNavigationVisible}
        style={{ "--navigation-width": `${desktopNavigationWidth}px` } as CSSProperties}
        className={`grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] ${
          desktopNavigationVisible
            ? "md:grid-cols-[var(--navigation-width)_minmax(0,1fr)]"
            : "md:grid-cols-[minmax(0,1fr)]"
        } ${
          inspector !== null && inspectorOpen
            ? desktopNavigationVisible
              ? "lg:grid-cols-[var(--navigation-width)_minmax(0,1fr)_17rem]"
              : "lg:grid-cols-[minmax(0,1fr)_17rem]"
            : ""
        }`}
      >

        {navigationOpen ? (
          <div
            aria-hidden="true"
            onClick={() => setNavigationOpen(false)}
            className="fixed inset-0 z-20 bg-canvas/70 md:hidden"
          />
        ) : null}
        <AppNavigation
          navigationId={navigationId}
          navigationOpen={navigationOpen}
          desktopVisible={desktopNavigationVisible}
          desktopWidth={desktopNavigationWidth}
          onDesktopWidthChange={resizeDesktopNavigation}
          navGroups={navGroups}
          section={section}
          sectionIcons={sectionIcons}
          sectionLabels={sectionLabels}
          onNavigate={(event, name) => {
            setNavigationOpen(false);
            followSectionLink(event, name);
          }}
          currentFace={currentFace}
          onFaceChange={setNavFace}
          consoles={consoles}
          orderedConsoles={orderedConsoles}
          activeConsole={activeConsole}
          onShowConsole={showConsole}
          onDuplicateConsole={(id) => void duplicateConsole(id)}
          onReorderConsoles={setConsoleOrder}
          onOpenShell={() => void openLocalShell()}
        />

        <main className="relative flex min-h-0 flex-col overflow-hidden">
          {connectionDraft !== null && (section === "Groups" || section === "Keys") ? (
            <div className="flex shrink-0 items-center gap-3 border-b border-notice-line bg-notice px-4 py-2 text-sm text-notice-ink">
              <p className="min-w-0 grow truncate">
                {t("conn.createDraftWaiting", { alias: connectionDraft.alias || t("conn.createUntitledDraft") })}
              </p>
              <Button className="shrink-0" onClick={() => navigate("Connections")}>
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
                      onNavigateForCreation: (target: CreationPrerequisite) => navigate(target),
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
                      onLock: () => setState("locked"),
                      onInspector: setInspector,
                      consoles,
                      onShowConsole: showConsole,
                      onTerminalSettingsChange: async (settings) => {
                        setTerminalSettings(settings);
                        await consoles.refresh();
                      },
                    }}
                    declared={{ groups, knownAliases }}
                  />
                </Suspense>
              ) : (
                <div className="h-full overflow-y-auto p-6">
                  <section aria-labelledby="not-found-heading" className="flex max-w-2xl flex-col gap-3">
                    <h2 id="not-found-heading" className="font-medium">{t("shell.pageNotFound")}</h2>
                    <p className="text-sm text-ink-muted">{t("shell.pageNotFoundDescription")}</p>
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
          <InspectorPane label={inspector.label}>{inspector.body}</InspectorPane>
        ) : null}
      </div>
    </div>
  );
}

type SectionViewProps = {
  section: Section;
  navigation: Navigation;
  handoff: Handoff;
  shell: Shell;
  declared: Declared;
};

function SectionView(props: SectionViewProps) {
  if (props.section === "Terminal") {
    return null;
  }
  if (props.section === "Connections") {
    return (
      <ConnectionsPage
        onOpenFile={props.navigation.onOpenFile}
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
  return <div className="h-full overflow-y-auto p-6">{<PaddedSection {...props} />}</div>;
}

function TerminalScreen({
  consoles,
  activeConsole,
  settings,
  hostAppearance,
}: {
  consoles: TerminalSessionsState;
  activeConsole: string | null;
  settings: TerminalSettings;
  hostAppearance: Map<string, TerminalAppearance>;
}) {
  const t = useTranslate();
  const session = consoles.sessions.find((entry) => entry.id === activeConsole);
  const appearance = resolveAppearance(
    session?.alias === undefined ? undefined : hostAppearance.get(session.alias),
    settings.appearance,
  );
  if (session === undefined) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <section className="sshc-card w-full max-w-md rounded-2xl bg-card p-8 text-center" role="status">
          <span
            aria-hidden="true"
            className="mx-auto grid size-12 place-items-center rounded-xl bg-accent font-mono text-sm font-bold text-accent-ink shadow-sm"
          >
            &gt;_
          </span>
          <h2 className="mt-4 text-xl font-semibold tracking-tight text-ink">{t("terminal.emptyHeading")}</h2>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-ink-muted">{t("terminal.emptyHint")}</p>
          <div aria-hidden="true" className="mx-auto mt-5 max-w-xs rounded-lg bg-term-bg px-4 py-3 text-left font-mono text-xs text-ink shadow-inner">
            <span className="text-live">$</span> ssh host
            <span className="ml-1 inline-block h-3 w-1.5 translate-y-0.5 bg-ink" />
          </div>
        </section>
      </div>
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      <Suspense fallback={<RouteSkeleton kind="terminal" />}>

        <TerminalView
          key={session.id}
          session={session}
          {...(settings.fontSize === undefined ? {} : { fontSize: settings.fontSize })}
          {...(appearance.palette === "" ? {} : { palette: appearance.palette })}
          {...(appearance.font === "" ? {} : { font: appearance.font })}
          {...(appearance.background === "" ? {} : { background: appearance.background })}
          {...(appearance.tint === undefined ? {} : { tint: appearance.tint })}
          copyOnSelect={settings.copyOnSelect ?? true}
          rightClickPaste={settings.rightClickPaste ?? true}
          onExit={() => consoles.markExited(session.id)}
        />
      </Suspense>
    </div>
  );
}

function PaddedSection({ section, navigation, handoff, shell, declared }: SectionViewProps) {
  const { fileTarget, onNavigate, onNavigateLocation } = navigation;
  const { onLock, onInspector, consoles, onShowConsole, onTerminalSettingsChange } = shell;
  if (section === "Home") {
    return (
      <OverviewPanel
        onNavigate={onNavigate}
        onNavigateLocation={onNavigateLocation}
        onConsoleOpened={onShowConsole}
      />
    );
  }
  if (section === "Config") {
    return <ConfigExplorer target={fileTarget} />;
  }
  if (section === "Groups") {
    return <GroupsPanel onInspector={onInspector} />;
  }
  if (section === "Secrets") {
    return <SecretsPanel onLock={onLock} />;
  }
  if (section === "Settings") {
    return <SettingsPanel consoles={consoles} onTerminalSettingsChange={onTerminalSettingsChange} />;
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
      <h2 id="section-heading" className="font-medium">{section}</h2>
    </section>
  );
}

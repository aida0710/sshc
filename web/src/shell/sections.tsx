import { Suspense, lazy } from "react";
import { OverviewPanel } from "../overview/OverviewPanel";
import type { Section } from "../routing/sectionRoute";
import { parseSettingsPage } from "../settings/settingsRoute";
import type { SFTPTarget } from "../sftp/SFTPPanel";
import { RouteSkeleton } from "../ui/RouteSkeleton";
import { SectionBar } from "../ui/SectionBar";
import { useTranslate } from "../i18n/context";
import { menuColumns } from "./menuEntries";
import { MenuPanel } from "./MenuPanel";
import { sectionLabels } from "./sectionPresentation";
import type { Declared, Handoff, Navigation, Shell } from "./sectionProps";

const AboutPage = lazy(() => import("../about/AboutPage").then((module) => ({ default: module.AboutPage })));
const LicensePage = lazy(() => import("../licenses/LicensePage").then((module) => ({ default: module.LicensePage })));

const ConnectionsPage = lazy(() =>
  import("../connections/ConnectionsPage").then(({ ConnectionsPage }) => ({
    default: ConnectionsPage,
  })),
);
const ConfigExplorer = lazy(() =>
  import("../explorer/ConfigExplorer").then(({ ConfigExplorer }) => ({
    default: ConfigExplorer,
  })),
);
const GroupsPanel = lazy(() =>
  import("../groups/GroupsPanel").then(({ GroupsPanel }) => ({
    default: GroupsPanel,
  })),
);
const HistoryPanel = lazy(() =>
  import("../history/HistoryPanel").then(({ HistoryPanel }) => ({
    default: HistoryPanel,
  })),
);
const KeysScreen = lazy(() =>
  import("../keys/KeysScreen").then(({ KeysScreen }) => ({
    default: KeysScreen,
  })),
);
const DiagnosticsPanel = lazy(() =>
  import("../diagnostics/DiagnosticsPanel").then(({ DiagnosticsPanel }) => ({
    default: DiagnosticsPanel,
  })),
);
const SecretsPanel = lazy(() =>
  import("../secrets/SecretsPanel").then(({ SecretsPanel }) => ({
    default: SecretsPanel,
  })),
);
const SettingsPanel = lazy(() =>
  import("../settings/SettingsPanel").then(({ SettingsPanel }) => ({
    default: SettingsPanel,
  })),
);
const VPNPanel = lazy(() =>
  import("../vpn/VPNPanel").then(({ VPNPanel }) => ({ default: VPNPanel })),
);
const SyncPanel = lazy(() =>
  import("../sync/SyncPanel").then(({ SyncPanel }) => ({ default: SyncPanel })),
);
const KnownHostsPanel = lazy(() =>
  import("../knownhosts/KnownHostsPanel").then(({ KnownHostsPanel }) => ({
    default: KnownHostsPanel,
  })),
);
const RemoteKeyPanel = lazy(() =>
  import("../remotekeys/RemoteKeyPanel").then(({ RemoteKeyPanel }) => ({
    default: RemoteKeyPanel,
  })),
);
const SFTPWorkspace = lazy(() =>
  import("../sftp/SFTPWorkspace").then(({ SFTPWorkspace }) => ({
    default: SFTPWorkspace,
  })),
);
const SnippetsPanel = lazy(() =>
  import("../snippets/SnippetsPanel").then(({ SnippetsPanel }) => ({
    default: SnippetsPanel,
  })),
);

export const startSections: Section[] = ["Home", "Connections", "Files"];

// The id the header's toggle points at with aria-controls.
export const navigationId = "primary-navigation";

// Which section fills the main pane. The terminal is not a section of its
// own: it stays mounted behind whatever is shown, so it is drawn elsewhere.
export type SectionViewProps = {
  section: Section;
  navigation: Navigation;
  handoff: Handoff;
  shell: Shell;
  declared: Declared;
  sftpTarget: SFTPTarget | null;
  onSftpTargetHandled: (request: number) => void;
};

export function SectionView(props: SectionViewProps) {
  const t = useTranslate();
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
        onOpenSSHSession={props.shell.onOpenSSHSession}
      />
    );
  }
  // Home・Connections・SFTP は、上に共通の細い帯（SectionBar）を置き、その下を画面いっぱいに使う。
  if (props.section === "Home") {
    return (
      <OverviewPanel
        onNavigate={props.navigation.onNavigate}
        onNavigateLocation={props.navigation.onNavigateLocation}
        onSessionOpened={props.shell.onShowSession}
      />
    );
  }
  if (props.section === "Files") {
    // 帯は比較のボタンと一緒に SFTPWorkspace が描く。SFTPWorkspace を読み込むあいだも帯は出しておく。
    // Connections と同じく余白を取らず、帯・タブ・一覧・転送マネージャーの境目は線だけにする。
    return (
      <div className="h-full min-h-0 overflow-hidden">
        <Suspense fallback={<div className="flex h-full min-h-0 flex-col"><SectionBar title={t(sectionLabels.Files)} /><RouteSkeleton /></div>}>
          <PaddedSection {...props} />
        </Suspense>
      </div>
    );
  }
  if (props.section === "Menu") {
    // The Menu keeps its own margins: 8px around its card on a wide screen, as in the design.
    return (
      <div className="h-full overflow-y-auto">
        <MenuPanel columns={menuColumns} onNavigate={props.navigation.onNavigateLocation} />
      </div>
    );
  }
  return (
    <div className="h-full overflow-y-auto p-4 md:p-5">
      {<PaddedSection {...props} />}
    </div>
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
  const t = useTranslate();
  const { fileTarget, onNavigateLocation } = navigation;
  const {
    onLock,
    onVaultChanged,
    passwordless,
    onInspector,
    terminalSessions,
    onOpenTerminal,
    onTerminalSettingsChange,
  } = shell;
  if (section === "About") return <AboutPage />;
  if (section === "License") return <LicensePage />;
  if (section === "Config") {
    return (
      <ConfigExplorer
        target={fileTarget}
        onTargetHandled={navigation.onFileTargetHandled}
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
      />
    );
  }
  if (section === "Files") {
    return (
      <SFTPWorkspace
        title={t(sectionLabels.Files)}
        aliases={declared.knownAliases}
        hosts={declared.hosts}
        hostVPN={declared.hostVPN}
        target={sftpTarget}
        onTargetHandled={onSftpTargetHandled}
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
        onOpenTerminal={onOpenTerminal}
      />
    );
  }
  if (section === "Snippets") {
    return (
      <SnippetsPanel
        aliases={declared.knownAliases}
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
        selectedSnippetId={new URLSearchParams(navigation.location.search).get(
          "snippet",
        )}
      />
    );
  }
  if (section === "Groups") {
    return (
      <GroupsPanel
        onInspector={onInspector}
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
      />
    );
  }
  if (section === "Passwords" || section === "Key Passphrases" || section === "OTP") {
    const kind = section === "Passwords"
      ? "password"
      : section === "Key Passphrases"
        ? "key_passphrase"
        : "totp";
    return <SecretsPanel kind={kind} onLock={onLock} />;
  }
  if (section === "Settings") {
    return (
      <SettingsPanel
        page={parseSettingsPage(navigation.location.pathname) ?? "Engine"}
        terminalSessions={terminalSessions}
        onTerminalSettingsChange={onTerminalSettingsChange}
        onVaultChanged={onVaultChanged}
        passwordless={passwordless}
        onNavigatePage={(path) => navigation.onNavigateLocation(path)}
      />
    );
  }
  if (section === "Sync") {
    return <SyncPanel />;
  }
  if (section === "VPN") {
    return (
      <VPNPanel
        onNavigationBlockerChange={navigation.onNavigationBlockerChange}
        onNavigateLocation={onNavigateLocation}
      />
    );
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

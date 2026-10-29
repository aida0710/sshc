import { Suspense, lazy } from "react";
import { OverviewPanel } from "../overview/OverviewPanel";
import type { Section } from "../routing/sectionRoute";
import { parseSettingsPage } from "../settings/settingsRoute";
import type { SFTPTarget } from "../sftp/SFTPPanel";
import { PageBoard } from "../ui/PageBoard";
import { RouteSkeleton } from "../ui/RouteSkeleton";
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

// pageScroller is what scrolls a page. On a phone the bottom bar floats over its end, so the end is padded
// to scroll clear of the bar, and scrolling something into view (a form opened in a row) stops above it.
const pageScroller = "h-full overflow-y-auto pb-(--mobile-navigation-inset) scroll-pb-(--mobile-navigation-inset)";

export function SectionView(props: SectionViewProps) {
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
  if (props.section === "Home") {
    // Home draws its own card, with its name and count in the heading.
    return (
      <div className={pageScroller}>
        <OverviewPanel
          onNavigate={props.navigation.onNavigate}
          onNavigateLocation={props.navigation.onNavigateLocation}
          onSessionOpened={props.shell.onShowSession}
        />
      </div>
    );
  }
  if (props.section === "Files" || props.section === "Config" || props.section === "Snippets") {
    // These workspaces draw their own cards, filling the pane like Connections. While one loads, an
    // empty card holds the place.
    return (
      <div className="h-full min-h-0 overflow-hidden">
        <Suspense fallback={<PageBoard fill><RouteSkeleton /></PageBoard>}>
          <SectionPage {...props} />
        </Suspense>
      </div>
    );
  }
  if (props.section === "Menu") {
    // The Menu keeps its own margins: 8px around its card on a wide screen, as in the design.
    return (
      <div className={pageScroller}>
        <MenuPanel columns={menuColumns} onNavigate={props.navigation.onNavigateLocation} onOpenSearch={props.navigation.onOpenSearch} />
      </div>
    );
  }
  if (props.section === "Settings") {
    // Settings draws its own card, with its list of pages beside it.
    return (
      <div className={pageScroller}>
        <SectionPage {...props} />
      </div>
    );
  }
  return (
    <div className={pageScroller}>
      <PageBoard>
        <SectionPage {...props} />
      </PageBoard>
    </div>
  );
}

function SectionPage({
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
    onShowInspector,
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
        onShowInspector={onShowInspector}
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

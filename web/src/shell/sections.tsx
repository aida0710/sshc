import { lazy } from "react";
import { OverviewPanel } from "../overview/OverviewPanel";
import { sectionPath, type Section } from "../routing/sectionRoute";
import { parseSettingsPage, settingsPageMeta, settingsPages } from "../settings/settingsRoute";
import type { IconName } from "../ui/icons";
import type { MessageKey } from "../i18n/messages";
import type { SFTPTarget } from "../sftp/SFTPPanel";
import { MenuPanel, type MenuGroup } from "./MenuPanel";
import type { Declared, Handoff, Navigation, Shell } from "./sectionProps";

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

export const sectionLabels: Record<Section, MessageKey> = {
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
  Passwords: "section.passwords",
  "Key Passphrases": "section.keyPassphrases",
  OTP: "section.otp",
  Settings: "section.settings",
  Sync: "section.sync",
  VPN: "section.vpn",
  History: "section.history",
  License: "section.license",
};

export const sectionIcons: Record<Section, IconName> = {
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
  Passwords: "secrets",
  "Key Passphrases": "keys",
  OTP: "secrets",
  Settings: "settings",
  Sync: "sync",
  VPN: "connections",
  History: "history",
  License: "inspector",
};

export const startSections: Section[] = ["Home", "Connections", "Files"];

// The id the header's toggle points at with aria-controls.
export const navigationId = "primary-navigation";

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
      "Snippets",
      "Settings",
      "Sync",
      "VPN",
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
    label: "shell.navVault",
    items: sectionMenuItems(["Passwords", "Key Passphrases", "OTP"]),
  },
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
      "Snippets",
      "Sync",
      "VPN",
      "History",
    ]),
  },
];


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
      ? "h-full overflow-hidden p-2 md:px-5 md:pb-5 md:pt-3"
      : "h-full overflow-y-auto p-4 md:p-5"}
    >
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
  const { fileTarget, onNavigate, onNavigateLocation } = navigation;
  const {
    onLock,
    onVaultChanged,
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
  if (section === "License") return <LicensePage />;
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
        hostVPN={declared.hostVPN}
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
        consoles={consoles}
        onTerminalSettingsChange={onTerminalSettingsChange}
        onVaultChanged={onVaultChanged}
      />
    );
  }
  if (section === "Sync") {
    return <SyncPanel />;
  }
  if (section === "VPN") {
    return <VPNPanel aliases={declared.knownAliases} />;
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

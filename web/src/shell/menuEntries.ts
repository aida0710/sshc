import type { MessageKey } from "../i18n/messages";
import { sectionPath, type Section } from "../routing/sectionRoute";
import { settingsPageMeta, settingsPages, type SettingsPage } from "../settings/settingsRoute";
import type { IconName } from "../ui/icons";
import { sectionIcons, sectionLabels } from "./sectionPresentation";

export type MenuItem = {
  key: string;
  label: MessageKey;
  // description says in a few words what the page is for; the Menu shows it under the name.
  description: MessageKey;
  icon: IconName;
  href: string;
};

export type MenuGroup = {
  label: MessageKey;
  items: MenuItem[];
};

function sectionItem(section: Section, description: MessageKey): MenuItem {
  return { key: section, label: sectionLabels[section], description, icon: sectionIcons[section], href: sectionPath(section) };
}

const settingsDescriptions: Record<SettingsPage, MessageKey> = {
  General: "menu.generalDescription",
  Appearance: "menu.appearanceDescription",
  Terminal: "menu.terminalDescription",
  SFTP: "menu.sftpDescription",
  Shortcuts: "menu.shortcutsDescription",
  Notifications: "menu.notificationsDescription",
  Vault: "menu.vaultDescription",
  Machines: "menu.machinesDescription",
  Connections: "menu.connectionsDescription",
};

const configuration: MenuGroup = {
  label: "shell.navConnections",
  items: [sectionItem("Config", "menu.configDescription")],
};

const security: MenuGroup = {
  label: "shell.navKeysHosts",
  items: [
    sectionItem("Keys", "menu.keysDescription"),
    sectionItem("Known Hosts", "menu.knownHostsDescription"),
    sectionItem("Remote Keys", "menu.remoteKeysDescription"),
  ],
};

const vault: MenuGroup = {
  label: "shell.navVault",
  items: [
    sectionItem("Passwords", "menu.passwordsDescription"),
    sectionItem("Key Passphrases", "menu.keyPassphrasesDescription"),
    sectionItem("OTP", "menu.otpDescription"),
  ],
};

const settings: MenuGroup = {
  label: "section.settings",
  items: settingsPages.map((page) => ({
    key: page,
    label: settingsPageMeta[page].label,
    description: settingsDescriptions[page],
    icon: settingsPageMeta[page].icon,
    href: settingsPageMeta[page].path,
  })),
};

const tools: MenuGroup = {
  label: "shell.navMaintenance",
  items: [
    sectionItem("Diagnostics", "menu.diagnosticsDescription"),
    sectionItem("Snippets", "menu.snippetsDescription"),
    sectionItem("Sync", "menu.syncDescription"),
    sectionItem("VPN", "menu.vpnDescription"),
    sectionItem("History", "menu.historyDescription"),
  ],
};

const others: MenuGroup = {
  label: "menu.others",
  items: [sectionItem("License", "menu.licenseDescription"), sectionItem("About", "menu.aboutDescription")],
};

// menuColumns is the Menu's three columns on a wide screen, as in the design: what is kept in ~/.ssh,
// the settings, and the tools. Narrower screens show the same groups in this order.
export const menuColumns: MenuGroup[][] = [
  [configuration, security, vault],
  [settings],
  [tools, others],
];

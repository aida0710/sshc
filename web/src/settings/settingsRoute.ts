import type { MessageKey } from "../i18n/messages";
import type { IconName } from "../ui/icons";

export const settingsPages = [
  "General",
  "Appearance",
  "Terminal",
  "SFTP",
  "Shortcuts",
  "Notifications",
  "Vault",
  "Machines",
  "Connections",
] as const;

export type SettingsPage = (typeof settingsPages)[number];

// SettingsGroup is where a page sits in the list beside it: the global settings, the settings of the preset
// being edited, the machines, and what acts on this machine's sessions.
export type SettingsGroup = "global" | "preset" | "machines" | "actions";

export const settingsPageMeta: Record<
  SettingsPage,
  // description is only for what a person would not learn from the page itself. A line that restates what
  // the page shows is left out.
  { path: string; label: MessageKey; description?: MessageKey; icon: IconName; group: SettingsGroup }
> = {
  General: {
    path: "/settings/general",
    label: "settings.generalHeading",
    description: "settings.generalDescription",
    icon: "global",
    group: "global",
  },
  Appearance: {
    path: "/settings/appearance",
    label: "appearance.heading",
    icon: "appearance",
    group: "preset",
  },
  Terminal: {
    path: "/settings/terminal",
    label: "terminal.settingsHeading",
    description: "settings.presetDescription",
    icon: "terminal",
    group: "preset",
  },
  SFTP: {
    path: "/settings/sftp",
    label: "sftp.settingsHeading",
    description: "settings.sftpDescription",
    icon: "files",
    group: "preset",
  },
  Shortcuts: {
    path: "/settings/shortcuts",
    label: "shortcuts.heading",
    description: "shortcuts.description",
    icon: "keyboard",
    group: "preset",
  },
  Notifications: {
    path: "/settings/notifications",
    label: "terminal.browserNotificationsHeading",
    icon: "notification",
    group: "preset",
  },
  Vault: {
    path: "/settings/vault",
    label: "settings.vaultHeading",
    icon: "secrets",
    group: "preset",
  },
  Machines: {
    path: "/settings/machines",
    label: "settings.machinesHeading",
    description: "settings.machinesDescription",
    icon: "engine",
    group: "machines",
  },
  Connections: {
    path: "/settings/connections",
    label: "desktop.closeAllHeading",
    icon: "disconnect",
    group: "actions",
  },
};

export function settingsPagePath(page: SettingsPage): string {
  return settingsPageMeta[page].path;
}

export function parseSettingsPage(pathname: string): SettingsPage | null {
  return settingsPages.find((page) => {
    const path = settingsPagePath(page);
    return pathname === path || pathname === `${path}/`;
  }) ?? null;
}

export function canonicalSettingsPath(pathname: string): string | null {
  const page = parseSettingsPage(pathname);
  return page === null ? null : settingsPagePath(page);
}

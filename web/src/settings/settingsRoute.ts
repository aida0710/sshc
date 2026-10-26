import type { MessageKey } from "../i18n/messages";
import type { IconName } from "../ui/icons";

export const settingsPages = [
  "Appearance",
  "Engine",
  "Terminal",
  "SFTP",
  "Shortcuts",
  "Notifications",
  "Connections",
  "Password",
] as const;

export type SettingsPage = (typeof settingsPages)[number];

export const settingsPageMeta: Record<
  SettingsPage,
  // description is only for what a person would not learn from the page itself, such as that Engine's
  // settings stay on this machine. A line that restates what the page shows is left out.
  { path: string; label: MessageKey; description?: MessageKey; icon: IconName }
> = {
  Appearance: {
    path: "/settings/appearance",
    label: "appearance.heading",
    icon: "appearance",
  },
  Engine: {
    path: "/settings/engine",
    label: "engine.heading",
    description: "settings.engineDescription",
    icon: "engine",
  },
  Terminal: {
    path: "/settings/terminal",
    label: "terminal.settingsHeading",
    icon: "terminal",
  },
  SFTP: {
    path: "/settings/sftp",
    label: "sftp.settingsHeading",
    icon: "files",
  },
  Shortcuts: {
    path: "/settings/shortcuts",
    label: "shortcuts.heading",
    description: "shortcuts.description",
    icon: "keyboard",
  },
  Notifications: {
    path: "/settings/notifications",
    label: "terminal.browserNotificationsHeading",
    icon: "notification",
  },
  Connections: {
    path: "/settings/connections",
    label: "desktop.closeAllHeading",
    icon: "disconnect",
  },
  Password: {
    path: "/settings/password",
    label: "secrets.changeHeading",
    icon: "secrets",
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

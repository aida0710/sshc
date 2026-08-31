import type { MessageKey } from "../i18n/messages";
import type { IconName } from "../ui/icons";

export const settingsPages = [
  "Engine",
  "Terminal",
  "Notifications",
  "Connections",
  "Password",
] as const;

export type SettingsPage = (typeof settingsPages)[number];

export const settingsPageMeta: Record<
  SettingsPage,
  { path: string; label: MessageKey; description: MessageKey; icon: IconName }
> = {
  Engine: {
    path: "/settings/engine",
    label: "engine.heading",
    description: "settings.engineDescription",
    icon: "settings",
  },
  Terminal: {
    path: "/settings/terminal",
    label: "terminal.settingsHeading",
    description: "settings.terminalDescription",
    icon: "terminal",
  },
  Notifications: {
    path: "/settings/notifications",
    label: "terminal.browserNotificationsHeading",
    description: "settings.notificationsDescription",
    icon: "notification",
  },
  Connections: {
    path: "/settings/connections",
    label: "desktop.closeAllHeading",
    description: "settings.connectionsDescription",
    icon: "connections",
  },
  Password: {
    path: "/settings/password",
    label: "secrets.changeHeading",
    description: "settings.passwordDescription",
    icon: "secrets",
  },
};

export function settingsPagePath(page: SettingsPage): string {
  return settingsPageMeta[page].path;
}

export function parseSettingsPage(pathname: string): SettingsPage | null {
  if (pathname === "/settings" || pathname === "/settings/") return "Engine";
  return settingsPages.find((page) => {
    const path = settingsPagePath(page);
    return pathname === path || pathname === `${path}/`;
  }) ?? null;
}

export function canonicalSettingsPath(pathname: string): string | null {
  if (pathname === "/settings" || pathname === "/settings/") return "/settings";
  const page = parseSettingsPage(pathname);
  return page === null ? null : settingsPagePath(page);
}

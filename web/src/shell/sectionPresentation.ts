import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import type { IconName } from "../ui/icons";

// The name and icon of each section, as the navigation, the phone's tabs and the Menu show them.
export const sectionLabels: Record<Section, MessageKey> = {
  Home: "section.home",
  Menu: "section.menu",
  Connections: "section.connections",
  Terminal: "section.terminal",
  Files: "section.files",
  Snippets: "section.snippets",
  Config: "section.config",
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
  About: "section.about",
};

export const sectionIcons: Record<Section, IconName> = {
  Home: "home",
  Menu: "menu",
  Connections: "connections",
  Terminal: "terminal",
  Files: "files",
  Snippets: "snippets",
  Config: "config",
  Keys: "keys",
  "Known Hosts": "knownHosts",
  "Remote Keys": "remoteKeys",
  Diagnostics: "diagnostics",
  Passwords: "passwords",
  "Key Passphrases": "passphrases",
  OTP: "otp",
  Settings: "settings",
  Sync: "sync",
  VPN: "vpn",
  History: "history",
  License: "license",
  About: "info",
};

import { sections, type Section } from "../../src/routing/sectionRoute";

export type SectionNavigation = Readonly<{
  // The Primary navigation link, or the "Open <name>" link in Menu, that
  // reaches the section. Terminal has neither and is opened by its URL.
  navigation: string;
  // The heading the section shows once it is open.
  heading: string;
}>;

// Keyed by the app's own Section, so a new section fails the e2e typecheck
// until it is added here. The tests that walk every section then include it
// without editing a list of their own.
const navigationBySection: Record<Section, SectionNavigation> = {
  Home: { navigation: "Home", heading: "Your connections" },
  Menu: { navigation: "Menu", heading: "Menu" },
  Connections: { navigation: "Connections", heading: "Connections" },
  Terminal: { navigation: "Terminal", heading: "No session is open" },
  Files: { navigation: "SFTP", heading: "Remote files" },
  Snippets: { navigation: "Snippets", heading: "Snippets" },
  Config: { navigation: "Config", heading: "Configuration files" },
  Groups: { navigation: "Groups", heading: "Groups" },
  Keys: { navigation: "Keys", heading: "Keys" },
  "Known Hosts": { navigation: "Known Hosts", heading: "Known Hosts" },
  "Remote Keys": { navigation: "Install Key on Server", heading: "Install Key on Server" },
  Diagnostics: { navigation: "Ad hoc checks", heading: "Ad hoc checks" },
  Passwords: { navigation: "Account passwords", heading: "Account passwords" },
  "Key Passphrases": { navigation: "Key passphrases", heading: "Key passphrases" },
  OTP: { navigation: "OTP", heading: "One-time passwords (TOTP)" },
  Settings: { navigation: "Engine", heading: "Engine" },
  Sync: { navigation: "Sync", heading: "Remote sync" },
  VPN: { navigation: "VPN", heading: "VPN" },
  History: { navigation: "History", heading: "History" },
  License: { navigation: "License", heading: "License" },
};

export const everySection: readonly SectionNavigation[] = sections.map((section) => navigationBySection[section]);

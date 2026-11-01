import type { Section } from "../routing/sectionRoute";

// mobileNavigationSections are the sections a phone reaches in one tap from the bar at the bottom; the rest are
// under Menu.
export const mobileNavigationSections = ["Home", "Connections", "Files", "Terminal", "Menu"] as const satisfies readonly Section[];

// isMobileNavigationSection tells a section of the bar at the bottom from a page opened from Menu.
export function isMobileNavigationSection(section: Section): boolean {
  return (mobileNavigationSections as readonly Section[]).includes(section);
}

// barSection is the tab of the bar at the bottom a section belongs to. A page opened from Menu (Settings, SSH Keys,
// …) has no tab of its own, so it belongs to Menu.
export function barSection(section: Section): Section {
  return isMobileNavigationSection(section) ? section : "Menu";
}

import type { Section } from "../routing/sectionRoute";
import { isMobileNavigationSection } from "./mobileNavigationSections";

// keptWhenLeft tells a section kept mounted while another is shown: one of the phone's bottom bar, which a person
// switches between as an app's tabs. A page opened from the Menu is one step in, as a screen an app opens over
// its tab: leaving it closes it, and it opens fresh the next time, doing again what it does on opening (About
// checks for updates, Remote Keys picks the first key). The terminal is not drawn as a section; it stays
// mounted on its own.
export function keptWhenLeft(section: Section | null): section is Section {
  return section !== null && section !== "Terminal" && isMobileNavigationSection(section);
}

// nextKeptSections is the list of kept sections once the person moves from left to opened. A section left after
// a draft was guarded in it (see useKeptSections) is let go, so that a draft the person chose to discard is not
// there when the section is opened again.
export function nextKeptSections(
  kept: readonly Section[],
  { left, opened, letGoOfLeft }: { left: Section | null; opened: Section | null; letGoOfLeft: boolean },
): Section[] {
  const remaining = kept.filter((section) => !(letGoOfLeft && section === left));
  return keptWhenLeft(opened) && !remaining.includes(opened) ? [...remaining, opened] : remaining;
}

// drawnSections is what KeptSections draws: the kept sections, and the section shown when it is not one of them.
export function drawnSections(kept: readonly Section[], shown: Section | null): readonly Section[] {
  if (shown === null || shown === "Terminal" || kept.includes(shown)) return kept;
  return [...kept, shown];
}

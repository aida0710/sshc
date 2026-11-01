import { parseSectionPath, type Section } from "../routing/sectionRoute";
import type { BrowserLocation } from "../routing/useSectionRoute";
import { barSection, isMobileNavigationSection } from "./mobileNavigationSections";

// AndroidBackStep is what Android's Back does on a page once no dialog, menu or drawer is left to close: leave the
// app, go one entry back in the page's history, or open a section in place of this page.
export type AndroidBackStep =
  | { kind: "leave" }
  | { kind: "history" }
  | { kind: "section"; section: Section };

// androidBackStep follows Android's pattern for a bar of top-level sections. From the top of a section of the bar
// other than Home, Back goes to Home, however the person moved between the sections; from Home it leaves the app.
// Inside a section (an opened connection, a page opened from Menu) it goes one step back: to the previous entry,
// or, when the page has none (it was opened first), to the top of the section it belongs to.
export function androidBackStep(location: BrowserLocation, hasEntryBefore: boolean): AndroidBackStep {
  const route = parseSectionPath(location.pathname);
  const atTop = route.kind === "section" && route.canonical && location.search === "";
  if (atTop && route.section === "Home") return { kind: "leave" };
  if (atTop && isMobileNavigationSection(route.section)) return { kind: "section", section: "Home" };
  if (hasEntryBefore) return { kind: "history" };
  return { kind: "section", section: route.kind === "section" ? barSection(route.section) : "Home" };
}

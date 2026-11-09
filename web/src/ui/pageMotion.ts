import { flushSync } from "react-dom";
import { parseSectionPath } from "../routing/sectionRoute";
import type { BrowserLocation } from "../routing/useSectionRoute";
import { matchesMediaQuery, mobileViewportQuery } from "./useMediaQuery";
import { drawPageWhole } from "./backGesturePage";

// While a change of the page fades (pageMotion.css), <html> carries this attribute.
const pageChangingAttribute = "data-page-changing";
// Each change counts up, so that the end of a fade cut short by the next change leaves the next one's mark alone.
let changeCount = 0;

function movesOnThisScreen(): boolean {
  return typeof document.startViewTransition === "function"
    && document.documentElement.dataset.motion !== "reduce"
    && matchesMediaQuery(mobileViewportQuery);
}

function sectionOf(location: BrowserLocation) {
  const route = parseSectionPath(location.pathname);
  return route.kind === "section" ? route.section : null;
}

// presentLocationChange makes a change of the page shown on a phone fade, as an app's screens change (change
// publishes the new location). The browser takes a picture of the screen before and after, and pageMotion.css
// fades from one to the other. A wide screen, a browser without view transitions, or reduced motion changes the
// page at once.
//
// A page settling its own address (Connections opening on its list of servers) replaces the entry within its
// section with nothing new to see: it must not fade, nor cut short the fade that opened the page. A replace that
// opens another section (Android's Back opening Home) is a change of page like any other.
export function presentLocationChange(
  change: () => void,
  { from, to, replaced }: { from: BrowserLocation; to: BrowserLocation; replaced: boolean },
) {
  const samePage = from.pathname === to.pathname && from.search === to.search;
  const settlingAddress = replaced && sectionOf(from) === sectionOf(to);
  if (samePage || settlingAddress || !movesOnThisScreen()) {
    change();
    return;
  }
  const root = document.documentElement;
  const thisChange = ++changeCount;
  root.setAttribute(pageChangingAttribute, "");
  const transition = document.startViewTransition(() => {
    // A back gesture that brought the page here leaves the page drawn small (backGesturePage.ts); the picture before
    // the change keeps that, and the page after it is drawn whole.
    drawPageWhole();
    flushSync(change);
  });
  void transition.finished.finally(() => {
    if (changeCount === thisChange) root.removeAttribute(pageChangingAttribute);
  });
}

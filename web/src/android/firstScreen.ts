import { useEffect } from "react";
import { loadingPlaceholderSelector } from "../ui/loadingPlaceholder";

// The page tells the Android app once per load; a screen that shows a placeholder again later is not news.
const reportedWindows = new WeakSet<Window>();

function placeholderInSight(root: ParentNode): boolean {
  return Array.from(root.querySelectorAll<HTMLElement>(loadingPlaceholderSelector)).some((placeholder) =>
    // A kept section that is hidden (KeptSections) may hold one; only what can be seen counts.
    typeof placeholder.checkVisibility === "function" ? placeholder.checkVisibility() : true);
}

// useFirstScreenReport tells the Android app when the first screen is drawn with its contents, so that the app
// keeps its splash until then (FirstScreenWait) instead of showing a blank screen, a skeleton and a "reading"
// state one after another. settled is whether the app knows what to show: not starting, and its settings read.
// A page may still be reading its contents then; the report waits until no placeholder is in sight.
export function useFirstScreenReport(settled: boolean, target: Window = window) {
  useEffect(() => {
    const bridge = target.sshcAndroid;
    if (!settled || bridge === undefined || reportedWindows.has(target)) return;
    const reportWhenShown = () => {
      if (reportedWindows.has(target)) return true;
      if (placeholderInSight(target.document)) return false;
      reportedWindows.add(target);
      bridge.firstScreenShown();
      return true;
    };
    if (reportWhenShown()) return;
    const observer = new MutationObserver(() => {
      if (reportWhenShown()) observer.disconnect();
    });
    observer.observe(target.document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["data-loading-placeholder", "style"] });
    return () => observer.disconnect();
  }, [settled, target]);
}

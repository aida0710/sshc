import { useEffect } from "react";

// While active, asks the browser to confirm before the page is closed or
// reloaded, because something that lives only in this page would be lost.
// The browser shows its own wording; the page cannot choose it.
export function useBeforeUnloadWarning(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      // Older Chromium and WebView ask only when returnValue is set.
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => window.removeEventListener("beforeunload", warnBeforeUnload);
  }, [active]);
}

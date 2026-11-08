import { useEffect } from "react";
import { preloadSectionPages } from "./sections";

// usePreloadedSectionPages fetches the pages' code once the app is ready and the browser has nothing else to
// do, so that the first opening of a page does not wait on its code. Safari has no requestIdleCallback; it
// fetches after the current work instead.
export function usePreloadedSectionPages(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    // A page that could not be fetched now is fetched again when it is opened, as without preloading.
    const preload = () => void preloadSectionPages().catch(() => undefined);
    if (typeof window.requestIdleCallback === "function") {
      const idle = window.requestIdleCallback(preload);
      return () => window.cancelIdleCallback(idle);
    }
    const timer = window.setTimeout(preload, 0);
    return () => window.clearTimeout(timer);
  }, [ready]);
}

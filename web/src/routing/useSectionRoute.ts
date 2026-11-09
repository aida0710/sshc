import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  parseSectionPath,
  sectionPath,
  type Section,
  type SectionRoute,
} from "./sectionRoute";

export type BrowserLocation = {
  pathname: string;
  search: string;
};

export type NavigateLocationOptions = {
  replace?: boolean;
};

export type NavigationBlocker = (next: BrowserLocation) => boolean;

// LocationPresenter makes a change of the location shown: it calls change, which publishes the new location,
// and may wrap it (the phone's fade between pages). replaced is whether the change took the place of the current
// entry (a page settling its own address, Android's Back opening the top of a section).
export type LocationPresenter = (
  change: () => void,
  transition: { from: BrowserLocation; to: BrowserLocation; replaced: boolean },
) => void;

const presentAtOnce: LocationPresenter = (change) => change();

const navigationPointKey = "__sshcNavigationPoint";

function navigationPoint(state: unknown): number | null {
  if (typeof state !== "object" || state === null) return null;
  const point = (state as Record<string, unknown>)[navigationPointKey];
  return typeof point === "number" && Number.isSafeInteger(point) ? point : null;
}

function stateAtNavigationPoint(state: unknown, point: number): Record<string, unknown> {
  const previous = typeof state === "object" && state !== null && !Array.isArray(state)
    ? state as Record<string, unknown>
    : {};
  return { ...previous, [navigationPointKey]: point };
}

function readLocation(): BrowserLocation {
  return { pathname: window.location.pathname, search: window.location.search };
}

export function useSectionRoute({ present = presentAtOnce }: { present?: LocationPresenter } = {}): {
  route: SectionRoute;
  location: BrowserLocation;
  navigate: (section: Section) => boolean;
  navigateLocation: (url: string, options?: NavigateLocationOptions) => boolean;
  setNavigationBlocker: (blocker: NavigationBlocker | null) => void;
  // hasEntryBefore tells whether the page's history has an entry of sshc before this one; on the first entry,
  // going back would leave sshc (or, in the Android app, do nothing).
  hasEntryBefore: () => boolean;
  // currentLocation is the location last published. location follows it once the page is drawn there, which the
  // presenter may put off (the phone's fade draws the new page after a picture of the one before), so what acts on
  // where the page is now, as Android's Back does, reads this.
  currentLocation: () => BrowserLocation;
} {
  const [location, setLocation] = useState<BrowserLocation>(readLocation);
  const locationRef = useRef(location);
  const blockerRef = useRef<NavigationBlocker | null>(null);
  const navigationPointRef = useRef<number | null>(null);
  const restoringPointRef = useRef<number | null>(null);
  const route = useMemo(() => parseSectionPath(location.pathname), [location.pathname]);

  // present is a function that does not change between renders (a module's function), as publish is kept.
  const publish = useCallback((next: BrowserLocation, { replaced = false } = {}) => {
    const from = locationRef.current;
    locationRef.current = next;
    present(() => setLocation(next), { from, to: next, replaced });
  }, [present]);

  const setNavigationBlocker = useCallback((blocker: NavigationBlocker | null) => {
    blockerRef.current = blocker;
  }, []);

  useEffect(() => {
    let initialPoint = navigationPoint(window.history.state);
    if (initialPoint === null) {
      initialPoint = 0;
      window.history.replaceState(stateAtNavigationPoint(window.history.state, initialPoint), "");
    }
    navigationPointRef.current = initialPoint;

    const synchronize = () => {
      const current = readLocation();
      const currentPoint = navigationPoint(window.history.state);
      if (restoringPointRef.current !== null && currentPoint === restoringPointRef.current) {
        navigationPointRef.current = currentPoint;
        restoringPointRef.current = null;
        return;
      }
      const next = parseSectionPath(current.pathname);
      const candidate = next.kind === "section" && !next.canonical
        ? { pathname: next.canonicalPath, search: current.search }
        : current;
      if (blockerRef.current !== null && !blockerRef.current(candidate)) {
        const previous = locationRef.current;
        const previousPoint = navigationPointRef.current;
        if (previousPoint !== null && currentPoint !== null && previousPoint !== currentPoint) {
          restoringPointRef.current = previousPoint;
          window.history.go(previousPoint - currentPoint);
          return;
        }
        window.history.replaceState(
          stateAtNavigationPoint(window.history.state, previousPoint ?? 0),
          "",
          `${previous.pathname}${previous.search}`,
        );
        return;
      }
      if (currentPoint !== null) navigationPointRef.current = currentPoint;
      if (next.kind === "section" && !next.canonical) {
        window.history.replaceState(
          stateAtNavigationPoint(window.history.state, navigationPointRef.current ?? 0),
          "",
          `${next.canonicalPath}${current.search}`,
        );
        publish({ pathname: next.canonicalPath, search: current.search });
        return;
      }
      publish(current);
    };

    synchronize();
    window.addEventListener("popstate", synchronize);
    return () => window.removeEventListener("popstate", synchronize);
  }, [publish]);

  const navigate = useCallback((section: Section) => {
    const path = sectionPath(section);
    const nextLocation = { pathname: path, search: "" };
    if (blockerRef.current !== null && !blockerRef.current(nextLocation)) return false;
    const replaced = window.location.pathname === path;
    if (replaced) {
      if (window.location.search !== "" || window.location.hash !== "") {
        window.history.replaceState(
          stateAtNavigationPoint(window.history.state, navigationPointRef.current ?? 0),
          "",
          path,
        );
      }
    } else {
      const point = (navigationPointRef.current ?? 0) + 1;
      window.history.pushState(stateAtNavigationPoint(null, point), "", path);
      navigationPointRef.current = point;
    }
    publish(nextLocation, { replaced });
    return true;
  }, [publish]);

  const navigateLocation = useCallback((url: string, options: NavigateLocationOptions = {}) => {
    const target = new URL(url, window.location.origin);
    if (target.origin !== window.location.origin) throw new Error("external navigation is not allowed");
    const nextLocation = { pathname: target.pathname, search: target.search };
    if (blockerRef.current !== null && !blockerRef.current(nextLocation)) return false;
    const next = `${target.pathname}${target.search}`;
    const current = `${window.location.pathname}${window.location.search}`;
    if (next === current) return true;
    if (options.replace === true) {
      window.history.replaceState(
        stateAtNavigationPoint(window.history.state, navigationPointRef.current ?? 0),
        "",
        next,
      );
    } else {
      const point = (navigationPointRef.current ?? 0) + 1;
      window.history.pushState(stateAtNavigationPoint(null, point), "", next);
      navigationPointRef.current = point;
    }
    publish(nextLocation, { replaced: options.replace === true });
    return true;
  }, [publish]);

  const hasEntryBefore = useCallback(() => (navigationPointRef.current ?? 0) > 0, []);
  const currentLocation = useCallback(() => locationRef.current, []);

  return { route, location, navigate, navigateLocation, setNavigationBlocker, hasEntryBefore, currentLocation };
}

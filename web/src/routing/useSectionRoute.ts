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

export function useSectionRoute(): {
  route: SectionRoute;
  location: BrowserLocation;
  navigate: (section: Section) => boolean;
  navigateLocation: (url: string, options?: NavigateLocationOptions) => boolean;
  setNavigationBlocker: (blocker: NavigationBlocker | null) => void;
} {
  const [location, setLocation] = useState<BrowserLocation>(readLocation);
  const locationRef = useRef(location);
  const blockerRef = useRef<NavigationBlocker | null>(null);
  const navigationPointRef = useRef<number | null>(null);
  const restoringPointRef = useRef<number | null>(null);
  const route = useMemo(() => parseSectionPath(location.pathname), [location.pathname]);

  const publish = useCallback((next: BrowserLocation) => {
    locationRef.current = next;
    setLocation(next);
  }, []);

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
    if (window.location.pathname === path) {
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
    publish(nextLocation);
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
    publish(nextLocation);
    return true;
  }, [publish]);

  return { route, location, navigate, navigateLocation, setNavigationBlocker };
}

import { useCallback, useEffect, useMemo, useState } from "react";
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

function readLocation(): BrowserLocation {
  return { pathname: window.location.pathname, search: window.location.search };
}

export function useSectionRoute(): {
  route: SectionRoute;
  location: BrowserLocation;
  navigate: (section: Section) => void;
  navigateLocation: (url: string, options?: NavigateLocationOptions) => void;
} {
  const [location, setLocation] = useState<BrowserLocation>(readLocation);
  const route = useMemo(() => parseSectionPath(location.pathname), [location.pathname]);

  useEffect(() => {
    const synchronize = () => {
      const current = readLocation();
      const next = parseSectionPath(current.pathname);
      if (next.kind === "section" && !next.canonical) {
        window.history.replaceState(
          null,
          "",
          `${next.canonicalPath}${current.search}`,
        );
        setLocation({ pathname: next.canonicalPath, search: current.search });
        return;
      }
      setLocation(current);
    };

    synchronize();
    window.addEventListener("popstate", synchronize);
    return () => window.removeEventListener("popstate", synchronize);
  }, []);

  const navigate = useCallback((section: Section) => {
    const path = sectionPath(section);
    if (window.location.pathname === path) {
      if (window.location.search !== "" || window.location.hash !== "") {
        window.history.replaceState(null, "", path);
      }
    } else {
      window.history.pushState(null, "", path);
    }
    setLocation({ pathname: path, search: "" });
  }, []);

  const navigateLocation = useCallback((url: string, options: NavigateLocationOptions = {}) => {
    const target = new URL(url, window.location.origin);
    if (target.origin !== window.location.origin) throw new Error("external navigation is not allowed");
    const next = `${target.pathname}${target.search}`;
    const current = `${window.location.pathname}${window.location.search}`;
    if (next === current) return;
    if (options.replace === true) window.history.replaceState(null, "", next);
    else window.history.pushState(null, "", next);
    setLocation({ pathname: target.pathname, search: target.search });
  }, []);

  return { route, location, navigate, navigateLocation };
}

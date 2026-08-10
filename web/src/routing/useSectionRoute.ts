import { useCallback, useEffect, useState } from "react";
import {
  parseSectionPath,
  sectionPath,
  type Section,
  type SectionRoute,
} from "./sectionRoute";

function readRoute(): SectionRoute {
  return parseSectionPath(window.location.pathname);
}

export function useSectionRoute(): {
  route: SectionRoute;
  navigate: (section: Section) => void;
} {
  const [route, setRoute] = useState<SectionRoute>(readRoute);

  useEffect(() => {
    const synchronize = () => {
      const next = readRoute();
      if (next.kind === "section" && !next.canonical) {
        window.history.replaceState(
          null,
          "",
          `${next.canonicalPath}${window.location.search}`,
        );
        setRoute({ ...next, canonical: true });
        return;
      }
      setRoute(next);
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
    setRoute(parseSectionPath(path));
  }, []);

  return { route, navigate };
}

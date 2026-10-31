import { useEffect, useState, type RefObject } from "react";

// A phone remains a touch interface when rotated into landscape. A touch screen as large as a PC's is laid
// out as a PC. index.css repeats this condition in its mobile-layout variant and the 44px touch sizes;
// change them together.
export const mobileViewportQuery = "(max-width: 767px), (pointer: coarse) and (max-width: 1023px)";
// Two shapes of that phone layout on a screen at least 600px wide (Material's medium width), where the
// phone's bars would stretch or crowd the page. A tablet held upright keeps the navigation open at the left,
// as the session list beside the page. A phone on its side, under 480px tall (Material's compact height),
// moves the bottom bar to a rail at the left so the page keeps its height. index.css lays both out from
// the attributes App sets with them.
export const pinnedNavigationQuery = "(pointer: coarse) and (min-width: 600px) and (max-width: 1023px) and (min-height: 480px)";
export const navigationRailQuery = "(min-width: 600px) and (max-width: 767px) and (max-height: 479px), (pointer: coarse) and (min-width: 600px) and (max-width: 1023px) and (max-height: 479px)";

function matches(query: string): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia(query).matches;
}

export function useMediaQuery(query: string): boolean {
  const [matched, setMatched] = useState(() => matches(query));

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia(query);
    const update = () => setMatched(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [query]);

  return matched;
}

export function useCompactViewport(
  container: RefObject<HTMLElement | null>,
  minimumWidth = 680,
): boolean {
  const narrowViewport = useMediaQuery(mobileViewportQuery);
  const [narrowContainer, setNarrowContainer] = useState(false);

  useEffect(() => {
    const element = container.current;
    if (element === null) return;
    const update = () => setNarrowContainer(element.clientWidth > 0 && element.clientWidth < minimumWidth);
    update();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [container, minimumWidth]);

  return narrowViewport || narrowContainer;
}

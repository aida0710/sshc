import { useEffect, useState, type RefObject } from "react";

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
  const narrowViewport = useMediaQuery("(max-width: 767px)");
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

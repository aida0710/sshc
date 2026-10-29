import { useEffect, useState, type RefObject } from "react";

// useMoreToTheRight tells whether a row that scrolls sideways has more past its right edge, so the row can fade
// there and show that it goes on (a tab that is cut off reads as the last one otherwise).
export function useMoreToTheRight(row: RefObject<HTMLElement | null>): boolean {
  const [more, setMore] = useState(false);
  useEffect(() => {
    const element = row.current;
    if (element === null) return;
    // One pixel of slack: the widths are fractional and scrollLeft stops just short of the end.
    const update = () => setMore(element.scrollLeft + element.clientWidth < element.scrollWidth - 1);
    update();
    element.addEventListener("scroll", update, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    observer?.observe(element);
    return () => {
      element.removeEventListener("scroll", update);
      observer?.disconnect();
    };
  }, [row]);
  return more;
}

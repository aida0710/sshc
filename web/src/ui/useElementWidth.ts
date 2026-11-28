import { useLayoutEffect, useState, type RefObject } from "react";

// useElementWidth は、要素の幅（px、枠を含む）を返し、大きさが変わるたびに測り直す。
// 測る前と、表示されていないあいだは 0 を返す。描く前に測るので、測る前の幅で描いた画面は出ない。
export function useElementWidth(element: RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0);

  useLayoutEffect(() => {
    const target = element.current;
    if (target === null) return;
    const measure = () => setWidth(target.getBoundingClientRect().width);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(target);
    return () => observer.disconnect();
  }, [element]);

  return width;
}

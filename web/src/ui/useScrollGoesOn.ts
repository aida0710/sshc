import { useEffect, useState, type RefObject } from "react";

export type ScrollEdge = "right" | "bottom";

// scrollFade は、続きがあるあいだ、区画の端の 2rem をぼかす。切れた所が最後に見えないようにするため。
export const scrollFade: Record<ScrollEdge, string> = {
  right: "[mask-image:linear-gradient(to_right,black_calc(100%-2rem),transparent)]",
  bottom: "[mask-image:linear-gradient(to_bottom,black_calc(100%-2rem),transparent)]",
};

// useScrollGoesOn は、スクロールする区画が、指した端の先にまだ続きを持つか。横に流れるタブの列（右）や、下に
// 固定したボタンの帯の上で流れるフォーム（下）に使い、続きがあるあいだ端をぼかす（scrollFade）。
export function useScrollGoesOn(area: RefObject<HTMLElement | null>, edge: ScrollEdge): boolean {
  const [goesOn, setGoesOn] = useState(false);
  useEffect(() => {
    const element = area.current;
    if (element === null) return;
    // 1px の余裕を見る。幅と高さは小数で、スクロールは端の少し手前で止まるため。
    const update = () => setGoesOn(edge === "right"
      ? element.scrollLeft + element.clientWidth < element.scrollWidth - 1
      : element.scrollTop + element.clientHeight < element.scrollHeight - 1);
    update();
    element.addEventListener("scroll", update, { passive: true });
    // 中身が伸び縮みしても（欄の下に文が出るなど）区画そのものの大きさは変わらないので、子の大きさも見張る。
    const resized = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    const watch = () => {
      resized?.disconnect();
      resized?.observe(element);
      for (const child of Array.from(element.children)) resized?.observe(child);
    };
    watch();
    const childrenChanged = typeof MutationObserver === "undefined" ? null : new MutationObserver(() => {
      watch();
      update();
    });
    childrenChanged?.observe(element, { childList: true });
    return () => {
      element.removeEventListener("scroll", update);
      resized?.disconnect();
      childrenChanged?.disconnect();
    };
  }, [area, edge]);
  return goesOn;
}

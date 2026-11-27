import { useEffect, useState, type RefObject } from "react";

// stuckToTopClass は、useStuckToTop で見る要素に付ける sticky の置き方。上の端より 1px 上で張り付くので、張り付くと
// 要素の上の 1px がスクロールする枠の外に出る。
export const stuckToTopClass = "sticky -top-px";

// isClippedAtTop は、見えている部分の上の端が要素の上の端より下にあるか。下の端が切れているだけ（窓が低い）のときは
// 張り付いていない。isIntersecting は使わない。Chrome は、見える割合がしきい値（1）を下回ると false にするため。
function isClippedAtTop(entry: IntersectionObserverEntry): boolean {
  return entry.intersectionRatio > 0 && entry.intersectionRect.top > entry.boundingClientRect.top;
}

// useStuckToTop は、stuckToTopClass の要素が、スクロールする枠の上の端に張り付いているかを答える。張り付いている間だけ
// 下地を付け、スクロールする前の見た目を変えないために使う。root を渡さない IntersectionObserver は、窓だけでなく
// スクロールする枠が切る分も見るので、張り付いて枠の外に出た 1px が分かる。
export function useStuckToTop(ref: RefObject<HTMLElement | null>): boolean {
  const [stuck, setStuck] = useState(false);
  useEffect(() => {
    const element = ref.current;
    if (element === null || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      const latest = entries.at(-1);
      if (latest !== undefined) setStuck(isClippedAtTop(latest));
    }, { threshold: [1] });
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return stuck;
}

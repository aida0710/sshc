import { useCallback, useState } from "react";
import type { SplitArea } from "./geometry";

// useSplitArea は、分割表示の領域の広さ（px）を測って覚える。observeArea を領域の要素の ref に付ける。
// 測る前は null。ほかの画面を開いてターミナルの領域が隠れている間（0×0）は、前に測った広さのままにする。
// サイドバーから分割したときに、隠れている間の 0×0 で断らないため。
export function useSplitArea() {
  const [area, setArea] = useState<SplitArea | null>(null);
  const observeArea = useCallback((element: HTMLElement | null) => {
    if (element === null || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry === undefined) return;
      const { width, height } = entry.contentRect;
      if (width <= 0 || height <= 0) return;
      setArea((previous) => previous?.width === width && previous.height === height ? previous : { width, height });
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return { area, observeArea };
}

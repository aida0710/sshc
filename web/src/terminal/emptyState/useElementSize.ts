import { useEffect, useState, type RefObject } from "react";

export type ElementSize = { readonly width: number; readonly height: number };

// useElementSize is an element's size in whole CSS pixels, or null while it has none (not laid out, hidden).
export function useElementSize(element: RefObject<HTMLElement | null>): ElementSize | null {
  const [size, setSize] = useState<ElementSize | null>(null);
  useEffect(() => {
    const target = element.current;
    if (target === null || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.round(entry?.contentRect.width ?? 0);
      const height = Math.round(entry?.contentRect.height ?? 0);
      setSize((current) => {
        if (width === 0 || height === 0) return null;
        return current?.width === width && current.height === height ? current : { width, height };
      });
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, [element]);
  return size;
}

import { useState } from "react";
import { readStoredValue, writeStoredValue } from "./browserStorage";

export type StoredColumnWidth = {
  key: string;
  fallback: number;
  minimum: number;
  maximum: number;
};

// Width is a whole number of pixels inside [minimum, maximum]; anything
// unreadable becomes the fallback rather than a NaN-sized column.
export function clampColumnWidth(width: number, bounds: StoredColumnWidth): number {
  if (!Number.isFinite(width)) return bounds.fallback;
  return Math.min(bounds.maximum, Math.max(bounds.minimum, Math.round(width)));
}

export function readStoredColumnWidth(bounds: StoredColumnWidth): number {
  const stored = readStoredValue(bounds.key);
  if (stored === null || stored.trim() === "") return bounds.fallback;
  return clampColumnWidth(Number(stored), bounds);
}

// useStoredColumnWidth keeps one column width per browser. Layout preferences
// are conveniences, so a blocked localStorage silently falls back to the default.
export function useStoredColumnWidth(bounds: StoredColumnWidth): [number, (width: number) => void] {
  const [width, setWidth] = useState(() => readStoredColumnWidth(bounds));
  const change = (next: number) => {
    const clamped = clampColumnWidth(next, bounds);
    setWidth(clamped);
    writeStoredValue(bounds.key, String(clamped));
  };
  return [width, change];
}

import { useStoredValue } from "./useStoredValue";

// ColumnWidth is a column whose width is dragged, in px, and the browser storage key that remembers it.
export type ColumnWidth = {
  key: string;
  fallback: number;
  minimum: number;
  maximum: number;
};

// Width is a whole number of pixels inside [minimum, maximum]; anything
// unreadable becomes the fallback rather than a NaN-sized column.
export function clampColumnWidth(width: number, column: ColumnWidth): number {
  if (!Number.isFinite(width)) return column.fallback;
  return Math.min(column.maximum, Math.max(column.minimum, Math.round(width)));
}

// useColumnWidth keeps one column width in this browser. The width follows the pointer and is written as it
// moves; a width fits the screen in front of the user, so it is not synced.
export function useColumnWidth(column: ColumnWidth): [number, (width: number) => void] {
  const [width, setWidth] = useStoredValue({
    key: column.key,
    fallback: column.fallback,
    parse: (stored) => stored === null ? column.fallback : clampColumnWidth(Number(stored), column),
  });
  return [width, (next) => setWidth(clampColumnWidth(next, column))];
}

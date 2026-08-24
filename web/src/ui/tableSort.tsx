import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";

export type SortDirection = "ascending" | "descending";

export function nextSort<Key extends string>(
  currentKey: Key,
  currentDirection: SortDirection,
  requestedKey: Key,
): { key: Key; direction: SortDirection } {
  return {
    key: requestedKey,
    direction: currentKey === requestedKey && currentDirection === "ascending" ? "descending" : "ascending",
  };
}

export function compareText(left: string, right: string): number {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" });
}

export function ordered<T>(items: readonly T[], compare: (left: T, right: T) => number, direction: SortDirection): T[] {
  const sign = direction === "ascending" ? 1 : -1;
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => sign * compare(left.item, right.item) || left.index - right.index)
    .map(({ item }) => item);
}

export function SortableTableHeader<Key extends string>({
  column,
  activeColumn,
  direction,
  onSort,
  className = "",
  buttonClassName = "",
  children,
}: {
  column: Key;
  activeColumn: Key;
  direction: SortDirection;
  onSort: (column: Key) => void;
  className?: string;
  buttonClassName?: string;
  children: ReactNode;
}) {
  const t = useTranslate();
  const active = column === activeColumn;
  const nextDirection = active && direction === "ascending" ? "descending" : "ascending";
  return (
    <th scope="col" aria-sort={active ? direction : "none"} className={className}>
      <button
        type="button"
        onClick={() => onSort(column)}
        className={`inline-flex w-full items-center gap-1 text-inherit ${buttonClassName}`}
      >
        <span>{children}</span>
        <span aria-hidden="true" className={active ? "text-accent" : "text-ink-faint"}>
          {active ? (direction === "ascending" ? "↑" : "↓") : "↕"}
        </span>
        <span className="sr-only">{t(nextDirection === "ascending" ? "table.sortAscending" : "table.sortDescending")}</span>
      </button>
    </th>
  );
}

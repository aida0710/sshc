import type { ReactNode } from "react";
import { Icon } from "./icons";
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

type SortColumnProps<Key extends string> = {
  column: Key;
  activeColumn: Key;
  direction: SortDirection;
  onSort: (column: Key) => void;
  children: ReactNode;
};

// SortButton is a column's name that sorts by it, with an arrow saying how the rows are sorted now.
export function SortButton<Key extends string>({ column, activeColumn, direction, onSort, className = "", children }: SortColumnProps<Key> & { className?: string }) {
  const t = useTranslate();
  const active = column === activeColumn;
  const nextDirection = nextSort(activeColumn, direction, column).direction;
  return (
    <button
      type="button"
      onClick={() => onSort(column)}
      className={`inline-flex items-center gap-1 text-inherit transition-colors duration-150 hover:text-ink ${className}`}
    >
      <span>{children}</span>
      <Icon
        name={active ? (direction === "ascending" ? "arrowUp" : "arrowDown") : "arrowUpDown"}
        className={`size-3.5 ${active ? "text-accent" : "text-ink-faint"}`}
      />
      <span className="sr-only">{t(nextDirection === "ascending" ? "table.sortAscending" : "table.sortDescending")}</span>
    </button>
  );
}

export function SortableTableHeader<Key extends string>({
  column,
  activeColumn,
  direction,
  onSort,
  className = "",
  buttonClassName = "",
  children,
}: SortColumnProps<Key> & {
  className?: string;
  buttonClassName?: string;
}) {
  return (
    <th scope="col" aria-sort={column === activeColumn ? direction : "none"} className={className}>
      <SortButton column={column} activeColumn={activeColumn} direction={direction} onSort={onSort} className={`w-full ${buttonClassName}`}>{children}</SortButton>
    </th>
  );
}

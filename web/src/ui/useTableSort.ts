import { useCallback, useState } from "react";
import { compareText, nextSort, ordered, type SortDirection } from "./tableSort";

// A cell's value as its table sorts it: numbers by size, text as compareText
// orders it.
export type SortValue = string | number;

// useTableSort keeps the column a table is sorted by, for a table that owns
// its order. headerProps goes to every SortableTableHeader of the table.
// sorted orders rows by the value sortValue gives for the sorted column; a
// table passes the same function its cells display from, so the order
// follows what each cell shows.
export function useTableSort<Column extends string>(initialColumn: Column) {
  const [sort, setSort] = useState<{ key: Column; direction: SortDirection }>({
    key: initialColumn,
    direction: "ascending",
  });
  const onSort = useCallback(
    (column: Column) => setSort((current) => nextSort(current.key, current.direction, column)),
    [],
  );

  function sorted<Row>(rows: readonly Row[], sortValue: (row: Row, column: Column) => SortValue): Row[] {
    return ordered(
      rows,
      (left, right) => compareSortValues(sortValue(left, sort.key), sortValue(right, sort.key)),
      sort.direction,
    );
  }

  return {
    headerProps: { activeColumn: sort.key, direction: sort.direction, onSort },
    sorted,
  };
}

function compareSortValues(left: SortValue, right: SortValue): number {
  if (typeof left === "number" && typeof right === "number") return left - right;
  return compareText(String(left), String(right));
}

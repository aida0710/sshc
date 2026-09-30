import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useTableSort } from "./useTableSort";

type Row = { name: string; size: number };
type Column = "name" | "size";

const rows: Row[] = [
  { name: "file10", size: 9 },
  { name: "file2", size: 100 },
  { name: "File1", size: 20 },
];

function sortValue(row: Row, column: Column) {
  return column === "name" ? row.name : row.size;
}

describe("useTableSort", () => {
  it("starts ascending on the initial column, reading numbers inside text as numbers", () => {
    const { result } = renderHook(() => useTableSort<Column>("name"));

    expect(result.current.headerProps).toMatchObject({ activeColumn: "name", direction: "ascending" });
    expect(result.current.sorted(rows, sortValue).map((row) => row.name)).toEqual(["File1", "file2", "file10"]);
  });

  it("reverses the sorted column when it is chosen again and starts another column ascending", () => {
    const { result } = renderHook(() => useTableSort<Column>("name"));

    act(() => result.current.headerProps.onSort("name"));
    expect(result.current.headerProps.direction).toBe("descending");
    expect(result.current.sorted(rows, sortValue).map((row) => row.name)).toEqual(["file10", "file2", "File1"]);

    act(() => result.current.headerProps.onSort("size"));
    expect(result.current.headerProps).toMatchObject({ activeColumn: "size", direction: "ascending" });
    expect(result.current.sorted(rows, sortValue).map((row) => row.size)).toEqual([9, 20, 100]);
  });

  it("keeps rows with the same value in the order they came", () => {
    const { result } = renderHook(() => useTableSort<Column>("size"));
    const tied = [{ name: "b", size: 1 }, { name: "a", size: 1 }];

    expect(result.current.sorted(tied, sortValue).map((row) => row.name)).toEqual(["b", "a"]);
  });
});

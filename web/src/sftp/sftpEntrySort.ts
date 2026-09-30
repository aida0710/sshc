import { compareText, ordered, type SortDirection } from "../ui/tableSort";
import type { RemoteEntry } from "./api";

export type SFTPSort = "name" | "type" | "size" | "modified";
export type SFTPSortState = { key: SFTPSort; direction: SortDirection };

export function sortEntries(entries: RemoteEntry[], sort: SFTPSortState): RemoteEntry[] {
  return ordered(
    entries,
    (left, right) => {
      switch (sort.key) {
        case "type": return compareText(left.type, right.type);
        case "size": return left.size - right.size;
        case "modified": return Date.parse(left.modifiedAt) - Date.parse(right.modifiedAt);
        case "name": return compareText(left.name, right.name);
      }
    },
    sort.direction,
  );
}

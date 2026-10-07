import { compareText, ordered, type SortDirection } from "../ui/tableSort";
import type { RemoteEntry } from "./api";

export const sftpSortColumns = ["name", "type", "size", "modified", "uid", "gid"] as const;
export type SFTPSort = typeof sftpSortColumns[number];
export type SFTPSortState = { key: SFTPSort; direction: SortDirection };

export function sortEntries(entries: RemoteEntry[], sort: SFTPSortState): RemoteEntry[] {
  return ordered(
    entries,
    (left, right) => {
      switch (sort.key) {
        case "uid": return (left.uid ?? -1) - (right.uid ?? -1);
        case "gid": return (left.gid ?? -1) - (right.gid ?? -1);
        case "type": return compareText(left.type, right.type);
        case "size": return left.size - right.size;
        case "modified": return Date.parse(left.modifiedAt) - Date.parse(right.modifiedAt);
        case "name": return compareText(left.name, right.name);
      }
    },
    sort.direction,
  );
}

import type { RemoteEntry } from "./api";

// How an entry behaves when it is opened, previewed or downloaded: a symlink
// stands in for what it points to, and one whose target cannot be read
// behaves like any other unreadable entry.
export type EntryKind = "file" | "directory" | "other";

export function entryKind(entry: Pick<RemoteEntry, "type" | "targetType">): EntryKind {
  if (entry.type === "symlink") return entry.targetType ?? "other";
  return entry.type === "file" || entry.type === "directory" ? entry.type : "other";
}

// Engine-side copies, moves, puts and gets never follow links, so only a
// file or a directory named directly can be handed to them.
export function movable(entry: Pick<RemoteEntry, "type">): boolean {
  return entry.type === "file" || entry.type === "directory";
}

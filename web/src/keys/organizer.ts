import type { KeyItem, KeyLocationInput, RelocateKeyResponse } from "./api";

export function groupOfKeyPath(relativePath: string): string {
  const segments = relativePath.split("/");
  if (segments.length < 3 || segments[0] !== "keys") return "";
  return segments.slice(1, -1).join("/");
}

export type Folder = { kind: "all" } | { kind: "ungrouped" } | { kind: "group"; name: string };

export type MoveTarget = Exclude<Folder, { kind: "all" }>;

export type FolderRow = { folder: Folder; count: number; depth: number };

export function sameFolder(left: Folder, right: Folder): boolean {
  if (left.kind !== right.kind) return false;
  if (left.kind === "group" && right.kind === "group") return left.name === right.name;
  return true;
}

export function itemsInFolder(items: KeyItem[], folder: Folder): KeyItem[] {
  if (folder.kind === "all") return items;
  const wanted = folder.kind === "ungrouped" ? "" : folder.name;
  return items.filter((item) => groupOfKeyPath(item.relativePath) === wanted);
}

export function folderRows(items: KeyItem[], groups: string[]): FolderRow[] {
  const directCount = new Map<string, number>();
  for (const item of items) {
    const group = groupOfKeyPath(item.relativePath);
    directCount.set(group, (directCount.get(group) ?? 0) + 1);
  }
  const rows: FolderRow[] = [{ folder: { kind: "all" }, count: items.length, depth: 0 }];
  for (const name of [...groups].sort()) {
    rows.push({ folder: { kind: "group", name }, count: directCount.get(name) ?? 0, depth: name.split("/").length });
  }
  rows.push({ folder: { kind: "ungrouped" }, count: directCount.get("") ?? 0, depth: 0 });
  return rows;
}

const keyKinds = new Set(["private_key", "public_key", "certificate"]);

export type ListFilter = "keys" | "all";

export function shownItems(items: KeyItem[], filter: ListFilter): KeyItem[] {
  if (filter === "all") return items;
  return items.filter((item) => keyKinds.has(item.kind) || item.permissionRisk);
}

export type MoveOutcome = {
  moved: string[];
  unchanged: string[];
  blocked: { path: string; blockers: string[] }[];
  failed: string[];
};

type Relocate = (keyId: string, change: KeyLocationInput) => Promise<RelocateKeyResponse>;

export async function moveInto(relocate: Relocate, items: KeyItem[], target: MoveTarget): Promise<MoveOutcome> {
  const group = target.kind === "ungrouped" ? "" : target.name;
  const outcome: MoveOutcome = { moved: [], unchanged: [], blocked: [], failed: [] };

  for (const item of items) {
    const path = item.relativePath;
    if (groupOfKeyPath(path) === group) {
      outcome.unchanged.push(path);
      continue;
    }
    try {
      const response = await relocate(item.id, { group });
      if (response.blockers.length > 0) {
        outcome.blocked.push({ path, blockers: response.blockers });
        continue;
      }
      outcome.moved.push(path);
    } catch {
      outcome.failed.push(path);
    }
  }
  return outcome;
}

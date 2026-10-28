import type { KeyItem, KeyLocationInput, RelocateKeyResponse } from "./api";
import { groupDepth } from "../rules/groupPath";

export function groupOfKeyPath(relativePath: string): string {
  const segments = relativePath.split("/");
  if (segments.length < 3 || segments[0] !== "keys") return "";
  return segments.slice(1, -1).join("/");
}

// keyListPath is how the keys page names a file: its path under ~/.ssh, without the keys/ that every group
// is kept under (the line under the name says which group it is in).
export function keyListPath(relativePath: string): string {
  return groupOfKeyPath(relativePath) === "" ? relativePath : relativePath.slice("keys/".length);
}

// relocateStem は、鍵を移すときに名前の欄へ最初に入れる名前である。公開鍵と
// 証明書は、対になる秘密鍵と同じ名前になるように拡張子を外す。
export function relocateStem(item: KeyItem): string {
  const base = item.relativePath.split("/").pop() ?? item.relativePath;
  if (item.kind === "private_key") return base;
  for (const suffix of ["-cert.pub", ".pub"]) {
    if (base.endsWith(suffix) && base.length > suffix.length) return base.slice(0, -suffix.length);
  }
  return base;
}

// Folder is what the left list of the keys page opens: every key, the keys outside any group, one group,
// or the trash (which lists trash entries instead of keys).
export type Folder = { kind: "all" } | { kind: "ungrouped" } | { kind: "group"; name: string } | { kind: "trash" };

export type MoveTarget = Extract<Folder, { kind: "ungrouped" } | { kind: "group" }>;

export type FolderRow = { folder: Folder; count: number; depth: number };

export function sameFolder(left: Folder, right: Folder): boolean {
  if (left.kind !== right.kind) return false;
  if (left.kind === "group" && right.kind === "group") return left.name === right.name;
  return true;
}

export function itemsInFolder(items: KeyItem[], folder: Folder): KeyItem[] {
  if (folder.kind === "all") return items;
  if (folder.kind === "trash") return [];
  const wanted = folder.kind === "ungrouped" ? "" : folder.name;
  return items.filter((item) => groupOfKeyPath(item.relativePath) === wanted);
}

// folderRows are the entries of the left list. Each count is the number of rows the table shows for that
// entry: a key pair or a certificate is one row with its private key, as in keyItemGroups.
export function folderRows(items: KeyItem[], groups: string[], trashCount: number): FolderRow[] {
  const rowsIn = (folder: Folder) => keyItemGroups(itemsInFolder(items, folder)).length;
  const rows: FolderRow[] = [{ folder: { kind: "all" }, count: rowsIn({ kind: "all" }), depth: 0 }];
  for (const name of [...groups].sort()) {
    const folder: Folder = { kind: "group", name };
    rows.push({ folder, count: rowsIn(folder), depth: groupDepth(name) });
  }
  rows.push({ folder: { kind: "ungrouped" }, count: rowsIn({ kind: "ungrouped" }), depth: 0 });
  rows.push({ folder: { kind: "trash" }, count: trashCount, depth: 0 });
  return rows;
}

const keyKinds = new Set(["private_key", "public_key", "certificate"]);

export type ListFilter = "keys" | "all";

export function shownItems(items: KeyItem[], filter: ListFilter): KeyItem[] {
  if (filter === "all") return items;
  return items.filter((item) => keyKinds.has(item.kind) || item.permissionRisk);
}

export type KeyItemGroup = { primary: KeyItem; related: KeyItem[] };

function relatedFingerprint(item: KeyItem): string {
  if (item.kind === "public_key") return item.fingerprint;
  if (item.kind === "certificate") return item.certificate?.signedKeyFingerprint ?? "";
  return "";
}

export function keyItemGroups(items: KeyItem[]): KeyItemGroup[] {
  const privateByFingerprint = new Map<string, KeyItem>();
  for (const item of items) {
    if (item.kind === "private_key" && item.fingerprint !== "" && !privateByFingerprint.has(item.fingerprint)) {
      privateByFingerprint.set(item.fingerprint, item);
    }
  }

  const relatedByPrivate = new Map<string, KeyItem[]>();
  const relatedIds = new Set<string>();
  for (const item of items) {
    const fingerprint = relatedFingerprint(item);
    if (fingerprint === "") continue;
    const privateKey = privateByFingerprint.get(fingerprint);
    if (privateKey === undefined) continue;
    relatedByPrivate.set(privateKey.id, [...(relatedByPrivate.get(privateKey.id) ?? []), item]);
    relatedIds.add(item.id);
  }

  return items.flatMap((item) => {
    if (relatedIds.has(item.id)) return [];
    return [{ primary: item, related: relatedByPrivate.get(item.id) ?? [] }];
  });
}

export function includeKeyPairContext(items: KeyItem[], matched: KeyItem[]): KeyItem[] {
  const matchedIds = new Set(matched.map((item) => item.id));
  for (const group of keyItemGroups(items)) {
    const members = [group.primary, ...group.related];
    if (members.some((item) => matchedIds.has(item.id))) {
      for (const item of members) matchedIds.add(item.id);
    }
  }
  return items.filter((item) => matchedIds.has(item.id));
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

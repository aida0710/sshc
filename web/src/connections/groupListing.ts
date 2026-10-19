import type { Overview } from "../api/config";

// グループを選ぶ一覧（移動先、スマホのグループの一覧）に並べる 1 行。
export type GroupListing = {
  name: string;
  label: string;
  depth: number;
  // そのグループと、その下のグループにある接続先の数。
  count: number;
};

export function belongsToGroup(group: string, name: string): boolean {
  return group === name || group.startsWith(`${name}/`);
}

// listGroups は、宣言済みのグループを親が先になる名前順で返す。
export function listGroups(overview: Overview): GroupListing[] {
  const groups = overview.hosts.filter((host) => host.identity.alias !== "").map((host) => host.group ?? "");
  return overview.groups
    .map((group) => group.name)
    .sort((left, right) => left.localeCompare(right, "ja"))
    .map((name) => ({
      name,
      label: name.slice(name.lastIndexOf("/") + 1),
      depth: name.split("/").length - 1,
      count: groups.filter((group) => belongsToGroup(group, name)).length,
    }));
}

export function ungroupedCount(overview: Overview): number {
  return overview.hosts.filter((host) => host.identity.alias !== "" && (host.group ?? "") === "").length;
}

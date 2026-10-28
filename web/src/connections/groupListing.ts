import type { Overview } from "../api/config";
import { buildGroupTree, decorateHosts, type GroupNode } from "./connectionTreeIndex";

// グループを選ぶ一覧（移動先、スマホのグループの一覧）に並べる 1 行。
export type GroupListing = {
  name: string;
  label: string;
  depth: number;
  // そのグループと、その下のグループにある接続先の数。
  count: number;
};

// listGroups は、宣言済みのグループを、広い画面の左のグループの一覧と同じ順（親が先、兄弟は表示順）で返す。
export function listGroups(overview: Overview): GroupListing[] {
  const flatten = (nodes: readonly GroupNode[], depth: number): GroupListing[] => nodes.flatMap((node) => [
    { name: node.name, label: node.label, depth, count: node.descendantCount },
    ...flatten(node.children, depth + 1),
  ]);
  return flatten(buildGroupTree(overview, decorateHosts(overview)), 0);
}

export function ungroupedCount(overview: Overview): number {
  return overview.hosts.filter((host) => host.identity.alias !== "" && (host.group ?? "") === "").length;
}

import type { HostEntry } from "../api/config";
import { isGroupOrDescendant } from "../rules/groupPath";
import { belongsToGroup, type DecoratedHost, type GroupNode } from "./connectionTreeIndex";
import type { Scope } from "./ConnectionTree";

// 接続先の一覧が、接続先をどの順で、どの見出しの下に並べるか。一覧の表示と、まとめて開くときの順の両方がこれに従う。

export type SortOrder = "configured" | "name";

export type ListSection = {
  name: string;
  items: DecoratedHost[];
};

function labelFor(host: HostEntry): string {
  return host.identity.alias === "" ? `Host ${host.patterns.join(" ")}` : host.identity.alias;
}

// listedGroups は、一覧に見出しを出すグループを、親から子へたどる順に返す。接続先の無い隠したグループは出さない。
export function listedGroups(groupTree: readonly GroupNode[]): GroupNode[] {
  const listed: GroupNode[] = [];
  const visit = (node: GroupNode) => {
    if (!(node.hidden && node.descendantCount === 0)) listed.push(node);
    node.children.forEach(visit);
  };
  groupTree.forEach(visit);
  return listed;
}

// sortedForList は、items を一覧の並び順（名前順か、保存した順）に並べた写しを返す。
export function sortedForList(items: readonly DecoratedHost[], sortOrder: SortOrder): DecoratedHost[] {
  return [...items].sort((left, right) => {
    if (sortOrder === "name") return labelFor(left.host).localeCompare(labelFor(right.host), "ja");
    return left.order - right.order || left.sourceOrder - right.sourceOrder;
  });
}

// listSections は、並べた items を見出しごとに分ける。宣言したグループを groups の順に、そのあと宣言の無いグループを
// 名前順に、最後にグループの無い接続先を置く。scope の外の見出しは出さない。
export function listSections({ items, groups, scope }: {
  items: readonly DecoratedHost[];
  groups: readonly GroupNode[];
  scope: Scope;
}): ListSection[] {
  const exact = new Map<string, DecoratedHost[]>();
  for (const item of items) {
    const members = exact.get(item.group) ?? [];
    members.push(item);
    exact.set(item.group, members);
  }
  if (scope.kind === "ungrouped") {
    return [{ name: "", items: exact.get("") ?? [] }];
  }

  const declared = new Set(groups.map((group) => group.name));
  const sections = groups
    .filter((group) => scope.kind === "all" || isGroupOrDescendant(group.name, scope.name))
    .filter((group) => items.some((item) => belongsToGroup(item, group.name)))
    .map((group) => ({ name: group.name, items: exact.get(group.name) ?? [] }));
  const undeclared = [...exact.keys()]
    .filter((name) => name !== "" && !declared.has(name))
    .sort((left, right) => left.localeCompare(right, "ja"));
  sections.push(...undeclared.map((name) => ({ name, items: exact.get(name) ?? [] })));
  if (scope.kind === "all" && exact.has("")) {
    sections.push({ name: "", items: exact.get("") ?? [] });
  }
  return sections;
}

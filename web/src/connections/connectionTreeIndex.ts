import type { HostEntry, HostMetadata, Overview } from "../api/config";
import { groupBaseName, isGroupOrDescendant } from "../rules/groupPath";
import { duplicateAliasesOf, hostMetadataByIdentity, identityKey, nearestDeclaredParent } from "./connectionBrowser";

// Connections の画面の接続先とグループを overview から組み立てる。グループの一覧と接続の一覧が、
// それぞれ同じ overview から作る。

export type DecoratedHost = {
  host: HostEntry;
  key: string;
  projectionIdentity: string;
  projectionOrder: number;
  group: string;
  tags: string[];
  colour: string;
  os: HostMetadata["os"];
  order: number;
  sourceOrder: number;
  duplicateAlias: boolean;
};

export type GroupNode = {
  name: string;
  label: string;
  // グループに付けた色。Home のグループの点と同じ色で、フォルダの印に付ける。
  colour: string;
  hidden: boolean;
  order: number;
  sourceOrder: number;
  descendantCount: number;
  children: GroupNode[];
};

export function belongsToGroup(item: DecoratedHost, group: string): boolean {
  return isGroupOrDescendant(item.group, group);
}

function hostBlockIdentity(host: HostEntry): string {
  return JSON.stringify([
    host.file.absolute,
    host.line,
    host.identity.path,
    host.identity.alias,
  ]);
}

// decorateHosts は、名前のある接続先を、一覧で一緒に出すもの（色・OS・タグなど）と並べる。
// パターンの規則（Host * など）は名前が無いので入れない。
export function decorateHosts(overview: Overview): DecoratedHost[] {
  const metadata = hostMetadataByIdentity(overview.metadata.hosts);
  const duplicates = duplicateAliasesOf(overview.hosts);
  const projectionOccurrences = new Map<string, number>();
  return overview.hosts.flatMap((host, sourceOrder) => {
    if (host.identity.alias === "") return [];
    const blockIdentity = hostBlockIdentity(host);
    const projectionOccurrence = projectionOccurrences.get(blockIdentity) ?? 0;
    projectionOccurrences.set(blockIdentity, projectionOccurrence + 1);
    const display = metadata.get(identityKey(host.identity));
    return [{
      host,
      key: identityKey(host.identity),
      // Include traversal may project the same physical Host block more than once.
      // Retain that occurrence in the render identity so display sorting/filtering
      // cannot collapse React siblings back onto the file-and-line identity.
      projectionIdentity: `${blockIdentity}\u0000${projectionOccurrence}`,
      projectionOrder: sourceOrder,
      group: host.group ?? "",
      tags: display?.tags ?? [],
      colour: display?.colour ?? "",
      os: display?.os || display?.detectedOS || "",
      order: display?.order ?? 0,
      sourceOrder,
      duplicateAlias: duplicates.has(host.identity.alias),
    }];
  });
}

// buildGroupTree は、宣言されたグループを、いちばん近い宣言された親の下に、保存された順で入れ子にする。
export function buildGroupTree(overview: Overview, decorated: readonly DecoratedHost[]): GroupNode[] {
  const declared = new Set(overview.groups.map((group) => group.name));
  const metadata = new Map((overview.metadata.groups ?? []).map((group) => [group.name, group]));
  const nodes = new Map<string, GroupNode>();
  overview.groups.forEach((group, sourceOrder) => {
    const display = metadata.get(group.name);
    nodes.set(group.name, {
      name: group.name,
      label: groupBaseName(group.name),
      colour: display?.colour ?? "",
      hidden: display?.hidden === true,
      order: display?.order ?? group.order ?? 0,
      sourceOrder,
      descendantCount: decorated.filter((item) => belongsToGroup(item, group.name)).length,
      children: [],
    });
  });
  const roots: GroupNode[] = [];
  for (const node of nodes.values()) {
    const parentName = nearestDeclaredParent(node.name, declared);
    const parent = parentName === "" ? undefined : nodes.get(parentName);
    if (parent === undefined) roots.push(node);
    else parent.children.push(node);
  }
  const sort = (nodesToSort: GroupNode[]) => {
    nodesToSort.sort((left, right) => left.order - right.order || left.sourceOrder - right.sourceOrder);
    nodesToSort.forEach((node) => sort(node.children));
  };
  sort(roots);
  return roots;
}

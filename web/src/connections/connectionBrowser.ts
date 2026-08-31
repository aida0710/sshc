import type { HostEntry, HostIdentity, Overview } from "../api/config";

export type BrowserServer = {
  host: HostEntry;
  identity: HostEntry["identity"];
  group: string;
  tags: string[];
  colour: string;
  order: number;
  duplicateAlias: boolean;
};

export type BrowserGroup = {
  name: string;
  label: string;
  parent: string;
  hidden: boolean;
  colour: string;
  order: number;
  descendantCount: number;
};

export type ConnectionBrowserIndex = {
  servers: BrowserServer[];
  groups: BrowserGroup[];
  duplicateAliases: ReadonlySet<string>;
  groupByName: ReadonlyMap<string, BrowserGroup>;
  visibleChildrenByParent: ReadonlyMap<string, BrowserGroup[]>;
};

export function identityKey(identity: HostIdentity): string {
  return `${identity.path}\u0000${identity.alias}`;
}

function nearestDeclaredParent(name: string, declared: ReadonlySet<string>): string {
  let candidate = name;
  while (true) {
    const cut = candidate.lastIndexOf("/");
    if (cut < 0) return "";
    candidate = candidate.slice(0, cut);
    if (declared.has(candidate)) return candidate;
  }
}

function belongsBelow(server: BrowserServer, group: string): boolean {
  return server.group === group || server.group.startsWith(`${group}/`);
}

export function duplicateAliasesOf(hosts: readonly HostEntry[]): ReadonlySet<string> {
  const counts = new Map<string, number>();
  for (const host of hosts) {
    if (host.identity.alias === "") continue;
    counts.set(host.identity.alias, (counts.get(host.identity.alias) ?? 0) + 1);
  }
  return new Set([...counts].filter(([, count]) => count > 1).map(([alias]) => alias));
}

export function buildConnectionBrowserIndex(overview: Overview): ConnectionBrowserIndex {
  const hostMetadata = new Map(
    (overview.metadata.hosts ?? []).map((entry) => [identityKey(entry.identity), entry]),
  );
  const duplicateAliases = duplicateAliasesOf(overview.hosts);
  const servers = overview.hosts
    .map((host, sourceOrder) => ({ host, sourceOrder }))
    .filter(({ host }) => host.identity.alias !== "")
    .map(({ host, sourceOrder }) => {
      const metadata = hostMetadata.get(identityKey(host.identity));
      return {
        host,
        identity: host.identity,
        group: host.group ?? "",
        tags: metadata?.tags ?? [],
        colour: metadata?.colour ?? "",
        order: metadata?.order ?? 0,
        duplicateAlias: duplicateAliases.has(host.identity.alias),
        sourceOrder,
      };
    })
    .sort((left, right) => left.order - right.order || left.sourceOrder - right.sourceOrder)
    .map(({ sourceOrder: _sourceOrder, ...server }) => server);

  const declaredNames = new Set(overview.groups.map((group) => group.name));
  const groupMetadata = new Map(
    (overview.metadata.groups ?? [])
      .filter((group) => declaredNames.has(group.name))
      .map((group) => [group.name, group]),
  );
  const groups = overview.groups
    .map((group, sourceOrder) => {
      const metadata = groupMetadata.get(group.name);
      const descendants = servers.filter((server) => belongsBelow(server, group.name));
      return {
        name: group.name,
        label: group.name.slice(group.name.lastIndexOf("/") + 1),
        parent: nearestDeclaredParent(group.name, declaredNames),
        hidden: metadata?.hidden === true,
        colour: metadata?.colour ?? group.colour ?? "",
        order: metadata?.order ?? group.order ?? 0,
        descendantCount: descendants.length,
        sourceOrder,
      };
    })
    .sort((left, right) => left.order - right.order || left.sourceOrder - right.sourceOrder)
    .map(({ sourceOrder: _sourceOrder, ...group }) => group);

  const groupByName = new Map(groups.map((group) => [group.name, group]));
  const directServersByGroup = new Map<string, BrowserServer[]>();
  directServersByGroup.set("", servers.filter((server) => server.group === ""));
  for (const group of groups) {
    directServersByGroup.set(
      group.name,
      servers.filter((server) => server.group === group.name),
    );
  }

  const directChildren = new Map<string, BrowserGroup[]>();
  for (const group of groups) {
    const children = directChildren.get(group.parent) ?? [];
    children.push(group);
    directChildren.set(group.parent, children);
  }
  const visibleChildrenByParent = new Map<string, BrowserGroup[]>();
  const visibleChildren = (parent: string): BrowserGroup[] => {
    const cached = visibleChildrenByParent.get(parent);
    if (cached !== undefined) return cached;
    const result = (directChildren.get(parent) ?? []).flatMap((group) => {
      const directServers = directServersByGroup.get(group.name) ?? [];
      return group.hidden && directServers.length === 0 ? visibleChildren(group.name) : [group];
    });
    visibleChildrenByParent.set(parent, result);
    return result;
  };
  visibleChildren("");
  for (const group of groups) visibleChildren(group.name);

  return {
    servers,
    groups,
    duplicateAliases,
    groupByName,
    visibleChildrenByParent,
  };
}

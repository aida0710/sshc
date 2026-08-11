export const hostEditorTabs = [
  "Basic",
  "Jump",
  "Advanced",
  "Raw",
  "Effective",
  "Diagnostics",
] as const;

export type HostEditorTab = (typeof hostEditorTabs)[number];

export type ConnectionArea = "Basic" | "Analysis" | "Advanced";
export type AdvancedArea = "Jump" | "Directives" | "Raw";

export function connectionAreaForTab(tab: HostEditorTab): {
  area: ConnectionArea;
  advanced: AdvancedArea;
} {
  switch (tab) {
    case "Basic":
    case "Diagnostics":
      return { area: "Basic", advanced: "Jump" };
    case "Effective":
      return { area: "Analysis", advanced: "Jump" };
    case "Jump":
      return { area: "Advanced", advanced: "Jump" };
    case "Advanced":
      return { area: "Advanced", advanced: "Directives" };
    case "Raw":
      return { area: "Advanced", advanced: "Raw" };
  }
}

export function checksExpandedForTab(tab: HostEditorTab): boolean {
  return tab === "Diagnostics";
}

export function tabForConnectionArea(
  area: ConnectionArea,
  advanced: AdvancedArea,
  checksExpanded: boolean,
): HostEditorTab {
  if (area === "Basic") return checksExpanded ? "Diagnostics" : "Basic";
  if (area === "Analysis") return "Effective";
  if (advanced === "Jump") return "Jump";
  if (advanced === "Directives") return "Advanced";
  return "Raw";
}

type LegacyConnectionTarget = {
  path: string;
  alias: string;
  tab: HostEditorTab;
};

export type ConnectionPanel = "Basic" | "Analysis" | "Advanced";

export type ConnectionBrowserLocation =
  | { view: "servers" }
  | { view: "groups"; scope: "root" }
  | { view: "groups"; scope: "named"; group: string }
  | { view: "groups"; scope: "ungrouped" };

export type ConnectionTarget = {
  path: string;
  alias: string;
  panel: ConnectionPanel;
  advanced: AdvancedArea;
};

export type ParsedConnectionLocation =
  | { kind: "redirect"; location: "/connections/servers" }
  | { kind: "invalid" }
  | {
      kind: "valid";
      browser: ConnectionBrowserLocation;
      target: ConnectionTarget | null;
    };

const tabSlugs: Record<HostEditorTab, string> = {
  Basic: "basic",
  Jump: "jump",
  Advanced: "advanced",
  Raw: "raw",
  Effective: "effective",
  Diagnostics: "diagnostics",
};

const tabsBySlug = new Map(
  Object.entries(tabSlugs).map(([tab, slug]) => [slug, tab as HostEditorTab]),
);

function safeRelativePath(path: string): boolean {
  if (path === "" || path.startsWith("/") || path.startsWith("~")) return false;
  if (path.includes("\0") || path.includes("\\")) return false;
  return path.split("/").every((segment) => segment !== "" && segment !== "." && segment !== "..");
}

function safeAlias(alias: string): boolean {
  return alias !== "" && !/[\p{Cc}]/u.test(alias);
}

export function parseConnectionSearch(search: string): LegacyConnectionTarget | null {
  const query = new URLSearchParams(search);
  const path = query.get("path");
  const alias = query.get("host");
  if (path === null && alias === null) return null;
  if (path === null || alias === null || !safeRelativePath(path) || !safeAlias(alias)) return null;
  return {
    path,
    alias,
    tab: tabsBySlug.get(query.get("tab") ?? "") ?? "Basic",
  };
}

const panelSlugs: Record<ConnectionPanel, string> = {
  Basic: "basic",
  Analysis: "analysis",
  Advanced: "advanced",
};

const panelsBySlug = new Map(
  Object.entries(panelSlugs).map(([panel, slug]) => [slug, panel as ConnectionPanel]),
);

const advancedSlugs: Record<AdvancedArea, string> = {
  Jump: "jump",
  Directives: "directives",
  Raw: "raw",
};

const advancedBySlug = new Map(
  Object.entries(advancedSlugs).map(([area, slug]) => [slug, area as AdvancedArea]),
);

const allowedQueryKeys = new Set(["scope", "path", "host", "panel", "advanced"]);

function hasOnlyUniqueAllowedKeys(query: URLSearchParams): boolean {
  const seen = new Set<string>();
  for (const key of query.keys()) {
    if (!allowedQueryKeys.has(key) || seen.has(key)) return false;
    seen.add(key);
  }
  return true;
}

function safeGroupSegment(segment: string): boolean {
  return (
    segment !== "" &&
    segment !== "." &&
    segment !== ".." &&
    !segment.includes("/") &&
    !segment.includes("\\") &&
    !/[\p{Cc}]/u.test(segment)
  );
}

function parseNamedGroup(rawPath: string): string | null {
  const rawSegments = rawPath.split("/");
  const segments: string[] = [];
  for (const rawSegment of rawSegments) {
    let segment: string;
    try {
      segment = decodeURIComponent(rawSegment);
    } catch {
      return null;
    }
    if (!safeGroupSegment(segment)) return null;
    segments.push(segment);
  }
  return segments.join("/");
}

function parseBrowserLocation(
  pathname: string,
  query: URLSearchParams,
): ConnectionBrowserLocation | null {
  const scope = query.get("scope");
  if (pathname === "/connections/servers") {
    return scope === null ? { view: "servers" } : null;
  }
  if (pathname === "/connections/groups") {
    if (scope === null) return { view: "groups", scope: "root" };
    return scope === "ungrouped" ? { view: "groups", scope: "ungrouped" } : null;
  }
  const prefix = "/connections/groups/";
  if (!pathname.startsWith(prefix) || scope !== null) return null;
  const group = parseNamedGroup(pathname.slice(prefix.length));
  return group === null ? null : { view: "groups", scope: "named", group };
}

function parseTarget(query: URLSearchParams): ConnectionTarget | null | false {
  const path = query.get("path");
  const alias = query.get("host");
  const panelSlug = query.get("panel");
  const advancedSlug = query.get("advanced");
  if (path === null && alias === null && panelSlug === null && advancedSlug === null) return null;
  if (
    path === null ||
    alias === null ||
    panelSlug === null ||
    !safeRelativePath(path) ||
    !safeAlias(alias)
  ) {
    return false;
  }
  const panel = panelsBySlug.get(panelSlug);
  if (panel === undefined) return false;
  if (panel !== "Advanced") {
    return advancedSlug === null
      ? { path, alias, panel, advanced: "Jump" }
      : false;
  }
  if (advancedSlug === null) return false;
  const advanced = advancedBySlug.get(advancedSlug);
  return advanced === undefined ? false : { path, alias, panel, advanced };
}

export function parseConnectionLocation(location: {
  pathname: string;
  search: string;
}): ParsedConnectionLocation {
  if (location.pathname === "/connections") {
    return { kind: "redirect", location: "/connections/servers" };
  }
  const query = new URLSearchParams(location.search);
  if (!hasOnlyUniqueAllowedKeys(query)) return { kind: "invalid" };
  const browser = parseBrowserLocation(location.pathname, query);
  const target = parseTarget(query);
  if (browser === null || target === false) return { kind: "invalid" };
  return { kind: "valid", browser, target };
}

function browserPath(browser: ConnectionBrowserLocation, query: URLSearchParams): string {
  if (browser.view === "servers") return "/connections/servers";
  if (browser.scope === "root") return "/connections/groups";
  if (browser.scope === "ungrouped") {
    query.set("scope", "ungrouped");
    return "/connections/groups";
  }
  const group = browser.group
    .split("/")
    .map((segment) => {
      if (!safeGroupSegment(segment)) throw new Error("Unsafe connection group");
      return encodeURIComponent(segment);
    })
    .join("/");
  return `/connections/groups/${group}`;
}

function currentConnectionLocation(
  browser: ConnectionBrowserLocation,
  target: ConnectionTarget | null,
): string {
  const query = new URLSearchParams();
  const pathname = browserPath(browser, query);
  if (target === null) {
    const search = query.toString();
    return search === "" ? pathname : `${pathname}?${search}`;
  }
  if (!safeRelativePath(target.path) || !safeAlias(target.alias)) {
    throw new Error("Unsafe connection target");
  }
  query.set("path", target.path);
  query.set("host", target.alias);
  query.set("panel", panelSlugs[target.panel]);
  if (target.panel === "Advanced") {
    query.set("advanced", advancedSlugs[target.advanced]);
  }
  return `${pathname}?${query.toString()}`;
}

function legacyConnectionLocation(target: LegacyConnectionTarget | null): string {
  if (target === null) return "/connections";
  const query = new URLSearchParams();
  query.set("path", target.path);
  query.set("host", target.alias);
  query.set("tab", tabSlugs[target.tab]);
  return `/connections?${query.toString()}`;
}

export function connectionLocation(target: LegacyConnectionTarget | null): string;
export function connectionLocation(
  browser: ConnectionBrowserLocation,
  target: ConnectionTarget | null,
): string;
export function connectionLocation(
  browserOrTarget: ConnectionBrowserLocation | LegacyConnectionTarget | null,
  target?: ConnectionTarget | null,
): string {
  if (arguments.length === 1) {
    return legacyConnectionLocation(browserOrTarget as LegacyConnectionTarget | null);
  }
  return currentConnectionLocation(browserOrTarget as ConnectionBrowserLocation, target ?? null);
}

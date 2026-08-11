export type AdvancedArea = "Jump" | "Directives" | "Raw";

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

function safeRelativePath(path: string): boolean {
  if (path === "" || path.startsWith("/") || path.startsWith("~")) return false;
  if (path.includes("\0") || path.includes("\\")) return false;
  return path.split("/").every((segment) => segment !== "" && segment !== "." && segment !== "..");
}

function safeAlias(alias: string): boolean {
  return alias !== "" && !/[\p{Cc}]/u.test(alias);
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

export function connectionLocation(
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

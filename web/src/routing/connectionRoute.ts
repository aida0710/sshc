export type AdvancedArea = "Jump" | "Forwards" | "Directives" | "Raw";

export type ConnectionPanel = "Basic" | "Analysis" | "Advanced" | "Sshc";

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
  Sshc: "sshc",
};

const panelsBySlug = new Map(
  Object.entries(panelSlugs).map(([panel, slug]) => [slug, panel as ConnectionPanel]),
);

const advancedSlugs: Record<AdvancedArea, string> = {
  Jump: "jump",
  Forwards: "port-forwarding",
  Directives: "directives",
  Raw: "raw",
};

const advancedBySlug = new Map(
  Object.entries(advancedSlugs).map(([area, slug]) => [slug, area as AdvancedArea]),
);

const allowedQueryKeys = new Set(["path", "host", "panel", "advanced"]);

function hasOnlyUniqueAllowedKeys(query: URLSearchParams): boolean {
  const seen = new Set<string>();
  for (const key of query.keys()) {
    if (!allowedQueryKeys.has(key) || seen.has(key)) return false;
    seen.add(key);
  }
  return true;
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
  const target = parseTarget(query);
  if (location.pathname !== "/connections/servers" || target === false) return { kind: "invalid" };
  return { kind: "valid", target };
}

export function connectionLocation(target: ConnectionTarget | null): string {
  const query = new URLSearchParams();
  if (target === null) {
    return "/connections/servers";
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
  return `/connections/servers?${query.toString()}`;
}

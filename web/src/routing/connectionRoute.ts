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

export type ConnectionTarget = {
  path: string;
  alias: string;
  tab: HostEditorTab;
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

export function parseConnectionSearch(search: string): ConnectionTarget | null {
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

export function connectionLocation(target: ConnectionTarget | null): string {
  if (target === null) return "/connections";
  const query = new URLSearchParams();
  query.set("path", target.path);
  query.set("host", target.alias);
  query.set("tab", tabSlugs[target.tab]);
  return `/connections?${query.toString()}`;
}

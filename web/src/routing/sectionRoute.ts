export const sections = [
  "Home",
  "Connections",
  "Terminal",
  "Files",
  "Snippets",
  "Config",
  "Groups",
  "Keys",
  "Known Hosts",
  "Remote Keys",
  "Diagnostics",
  "Secrets",
  "Settings",
  "Sync",
  "History",
] as const;

export type Section = (typeof sections)[number];

const paths: Record<Section, string> = {
  Home: "/",
  Connections: "/connections",
  Terminal: "/terminal",
  Files: "/files",
  Snippets: "/snippets",
  Config: "/config",
  Groups: "/groups",
  Keys: "/keys",
  "Known Hosts": "/known-hosts",
  "Remote Keys": "/install-key",
  Diagnostics: "/diagnostics",
  Secrets: "/secrets",
  Settings: "/settings",
  Sync: "/sync",
  History: "/history",
};

export type SectionRoute =
  | { kind: "section"; section: Section; canonicalPath: string; canonical: boolean }
  | { kind: "not-found"; pathname: string };

export function sectionPath(section: Section): string {
  return paths[section];
}

export function parseSectionPath(pathname: string): SectionRoute {
  for (const section of sections) {
    const canonicalPath = paths[section];
    if (pathname === canonicalPath) {
      return { kind: "section", section, canonicalPath, canonical: true };
    }
    if (canonicalPath !== "/" && pathname === `${canonicalPath}/`) {
      return { kind: "section", section, canonicalPath, canonical: false };
    }
  }
  if (pathname.startsWith("/connections/")) {
    return {
      kind: "section",
      section: "Connections",
      canonicalPath: paths.Connections,
      canonical: true,
    };
  }
  return { kind: "not-found", pathname };
}

import { useCallback, useEffect, useState } from "react";
import { configApi, type FileNode, type HostEntry, type HostMetadata, type Overview } from "../api/config";
import type { TerminalAppearance } from "../api/settings";
import type { Section } from "../routing/sectionRoute";

// What the SSH configuration declares, read once per section and when asked: the groups,
// the hosts (with the appearance and VPN route each one asked for) and the config files. Sections take these instead of reading the
// overview themselves.
export function useDeclaredConfig(enabled: boolean, section: Section | null) {
  const [groups, setGroups] = useState<string[]>([]);
  const [hostAppearance, setHostAppearance] = useState<Map<string, TerminalAppearance>>(new Map());
  const [hostVPN, setHostVPN] = useState<Map<string, string>>(new Map());
  const [knownAliases, setKnownAliases] = useState<string[]>([]);
  const [hosts, setHosts] = useState<HostEntry[]>([]);
  const [files, setFiles] = useState<FileNode[]>([]);
  const [readCount, setReadCount] = useState(0);

  const adoptOverview = useCallback((overview: Overview) => {
    setGroups((overview.metadata.groups ?? []).map((group) => group.name));
    const hostMetadata = overview.metadata.hosts ?? [];
    setHostAppearance(hostSettingByAlias(hostMetadata, (host) => host.appearance));
    setHostVPN(hostSettingByAlias(hostMetadata, (host) => host.vpn === "" ? undefined : host.vpn));
    setKnownAliases([
      ...new Set(
        overview.hosts
          .map((host) => host.identity.alias)
          .filter((alias) => alias !== ""),
      ),
    ]);
    setHosts(overview.hosts.filter((host) => host.identity.alias !== ""));
    setFiles(overview.files);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void configApi
      .overview()
      .then((overview) => {
        if (active) adoptOverview(overview);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [enabled, section, readCount, adoptOverview]);

  return {
    groups, hostAppearance, hostVPN, knownAliases, hosts, files,
    // adoptOverview takes an overview a section read after changing the connections, without leaving that section.
    adoptOverview,
    // readAgain is for a change made outside the sections.
    readAgain: () => setReadCount((count) => count + 1),
  };
}

// hostSettingByAlias は、alias の付いた接続のうち pick が値を返したものだけを、
// alias から値を引ける Map にする。値の無い接続は Map に入れず、既定の扱いにする。
function hostSettingByAlias<Setting>(
  hosts: HostMetadata[],
  pick: (host: HostMetadata) => Setting | undefined,
): Map<string, Setting> {
  return new Map(
    hosts.flatMap((host) => {
      const setting = pick(host);
      return setting === undefined || host.identity.alias === ""
        ? []
        : [[host.identity.alias, setting] as const];
    }),
  );
}

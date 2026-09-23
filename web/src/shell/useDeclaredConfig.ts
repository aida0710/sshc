import { useEffect, useState } from "react";
import { configApi, type FileNode, type HostEntry } from "../api/config";
import type { TerminalAppearance } from "../api/settings";
import type { Section } from "../routing/sectionRoute";

// What the SSH configuration declares, read once per section: the groups,
// the hosts (with the appearance, clipboard policy and VPN route each one
// asked for) and the config files. Sections take these instead of reading the
// overview themselves.
export function useDeclaredConfig(enabled: boolean, section: Section | null) {
  const [groups, setGroups] = useState<string[]>([]);
  const [hostAppearance, setHostAppearance] = useState<Map<string, TerminalAppearance>>(new Map());
  const [hostOSC52, setHostOSC52] = useState<Map<string, "allow" | "deny">>(new Map());
  const [hostVPN, setHostVPN] = useState<Map<string, string>>(new Map());
  const [knownAliases, setKnownAliases] = useState<string[]>([]);
  const [hosts, setHosts] = useState<HostEntry[]>([]);
  const [files, setFiles] = useState<FileNode[]>([]);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void configApi
      .overview()
      .then((overview) => {
        if (!active) return;
        setGroups((overview.metadata.groups ?? []).map((group) => group.name));
        setHostAppearance(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.appearance === undefined || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.appearance] as const],
            ),
          ),
        );
        setHostOSC52(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.osc52 === undefined || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.osc52] as const],
            ),
          ),
        );
        setHostVPN(
          new Map(
            (overview.metadata.hosts ?? []).flatMap((host) =>
              host.vpn === undefined || host.vpn === "" || host.identity.alias === ""
                ? []
                : [[host.identity.alias, host.vpn] as const],
            ),
          ),
        );
        setKnownAliases([
          ...new Set(
            overview.hosts
              .map((host) => host.identity.alias)
              .filter((alias) => alias !== ""),
          ),
        ]);
        setHosts(overview.hosts.filter((host) => host.identity.alias !== ""));
        setFiles(overview.files);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [enabled, section]);

  return { groups, hostAppearance, hostOSC52, setHostOSC52, hostVPN, knownAliases, hosts, files };
}

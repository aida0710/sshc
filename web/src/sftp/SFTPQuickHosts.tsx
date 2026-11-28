import { useEffect, useMemo, useState, type ReactNode } from "react";
import type { HostEntry } from "../api/config";
import type { RecentConnection } from "../api/recentConnections";
import { addressesByAlias } from "../connections/connectionAddress";
import { duplicateAliasesOf } from "../connections/connectionBrowser";
import { ConnectionCardBody, connectionCardButton, connectionCardClass, connectionCardGrid, connectionCardGroup, type LastConnected } from "../connections/ConnectionCard";
import { useLanguage } from "../i18n/context";
import type { HostLook } from "../shell/useDeclaredConfig";
import { useEngineApis } from "../shell/engineApis";
import { formatDateTime } from "../ui/format";
import { AccentTileIcon, OperatingSystemIcon } from "../ui/OperatingSystemIcon";
import { localHostAlias } from "./localHost";

// How many recent connections the empty pane offers besides this machine. More are in the picker.
const quickRecentCount = 4;

// SFTPQuickHosts は、空のペインから、SFTP が使う sshc エンジンのマシン（localhost）か、そのエンジンで最近使った
// 接続先を、接続先の選択を通さずに1回押して開く。今もある接続先だけを、Home と同じカードで並べる。
export function SFTPQuickHosts({ aliases, hosts, hostLook, onChoose, loadRecent: loadRecentOverride }: {
  aliases: readonly string[];
  hosts: readonly HostEntry[];
  hostLook: ReadonlyMap<string, HostLook>;
  onChoose: (alias: string) => void;
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
}) {
  const { t, locale } = useLanguage();
  const { recentConnections } = useEngineApis();
  const loadRecent = useMemo(() => loadRecentOverride ?? (() => recentConnections.recentConnections()), [loadRecentOverride, recentConnections]);
  const [recent, setRecent] = useState<RecentConnection[]>([]);
  useEffect(() => {
    let active = true;
    void loadRecent().then((loaded) => {
      if (active) setRecent(loaded.connections);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [loadRecent]);
  const addresses = useMemo(() => addressesByAlias(hosts), [hosts]);
  const groups = useMemo(() => new Map(hosts.map((host) => [host.identity.alias, host.group ?? ""])), [hosts]);
  const duplicateAliases = useMemo(() => duplicateAliasesOf(hosts), [hosts]);
  const known = new Set(aliases);
  const offered = recent.filter((connection) => known.has(connection.alias)).slice(0, quickRecentCount);
  return (
    <ul aria-label={t("sftp.quickHosts")} className={connectionCardGrid}>
      <QuickHostCard
        name={t("sftp.local.connection")}
        icon={<AccentTileIcon icon="home" />}
        lastConnected={null}
        address={t("sftp.local.engine")}
        onOpen={() => onChoose(localHostAlias)}
      />
      {offered.map((connection) => {
        const alias = connection.alias;
        const look = hostLook.get(alias);
        const at = formatDateTime(connection.lastConnectedAt, locale);
        return (
          <QuickHostCard
            key={alias}
            name={alias}
            icon={<OperatingSystemIcon os={look?.os} colour={look?.colour ?? ""} look="accentTile" />}
            lastConnected={{ at, label: t("connection.lastConnected", { at }) }}
            duplicateAlias={duplicateAliases.has(alias)}
            address={addresses.get(alias) ?? ""}
            group={groups.get(alias) ?? ""}
            onOpen={() => onChoose(alias)}
          />
        );
      })}
    </ul>
  );
}

// QuickHostCard は、1回押すとその接続先を開くカード。Home のカードと違い、選ぶだけの段は無い。
function QuickHostCard({ name, icon, lastConnected, duplicateAlias = false, address, group = "", onOpen }: {
  name: string;
  icon: ReactNode;
  lastConnected: LastConnected | null;
  duplicateAlias?: boolean;
  address: string;
  group?: string;
  onOpen: () => void;
}) {
  return (
    <li className={connectionCardClass(false)}>
      <button type="button" aria-label={name} onClick={onOpen} className={connectionCardButton}>
        <ConnectionCardBody
          icon={icon}
          name={name}
          lastConnected={lastConnected}
          duplicateAlias={duplicateAlias}
          address={address}
          end={group === "" ? null : <span className={connectionCardGroup} title={group}>{group}</span>}
        />
      </button>
    </li>
  );
}

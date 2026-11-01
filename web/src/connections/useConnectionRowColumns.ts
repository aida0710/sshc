import { useCallback, useMemo } from "react";
import type { HostEntry } from "../api/config";
import { recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { useLanguage } from "../i18n/context";
import { relativeDay } from "../ui/relativeDay";
import { useLoad } from "../ui/useLoad";
import type { RowColumns } from "./ConnectionRow";

const noRecentConnections: RecentConnectionList = { connections: [] };

// useConnectionRowColumns gives the rows of a list that has the whole width their columns: the group, and
// when the engine last connected to the host (今日 5:21, 3日前 14:02), read from the recent connections.
// While the list shares the page with what is open from it (enabled false), nothing is read.
export function useConnectionRowColumns(enabled: boolean): ((host: HostEntry, group: string) => RowColumns) | undefined {
  const { t, locale } = useLanguage();
  const loadRecent = useCallback(
    () => (enabled ? recentConnectionsApi.recentConnections() : Promise.resolve(noRecentConnections)),
    [enabled],
  );
  const recent = useLoad(loadRecent);
  const lastConnectedByAlias = useMemo(() => {
    const connections = recent.state === "loaded" ? recent.value.connections : [];
    return new Map(connections.map((connection) => [connection.alias, connection.lastConnectedAt]));
  }, [recent]);

  const columns = useCallback((host: HostEntry, group: string): RowColumns => {
    const at = lastConnectedByAlias.get(host.identity.alias);
    const time = at === undefined ? "" : new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit" }).format(new Date(at));
    return {
      group: group === "" ? t("tree.ungrouped") : group,
      lastConnected: at === undefined ? "" : t("connection.lastConnected", { at: `${relativeDay(at, new Date(), locale)} ${time}` }),
    };
  }, [lastConnectedByAlias, locale, t]);
  return enabled ? columns : undefined;
}

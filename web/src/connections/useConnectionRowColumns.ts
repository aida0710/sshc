import { useCallback, useMemo } from "react";
import type { HostEntry } from "../api/config";
import { recentConnectionsApi, type RecentConnectionList } from "../api/recentConnections";
import { useLanguage } from "../i18n/context";
import { formatDateTime } from "../ui/format";
import { useLoad } from "../ui/useLoad";
import type { RowColumns } from "./ConnectionRow";

const noRecentConnections: RecentConnectionList = { connections: [] };

// useConnectionRowColumns gives the rows of a list that has the whole width their columns: when the engine
// last connected to the host, read from the recent connections. While the list shares the page with what is
// open from it (enabled false), nothing is read.
export function useConnectionRowColumns(enabled: boolean): ((host: HostEntry) => RowColumns) | undefined {
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

  const columns = useCallback((host: HostEntry): RowColumns => {
    const at = lastConnectedByAlias.get(host.identity.alias);
    return { lastConnected: at === undefined ? "" : t("connection.lastConnected", { at: formatDateTime(at, locale) }) };
  }, [lastConnectedByAlias, locale, t]);
  return enabled ? columns : undefined;
}

import type { components } from "../../api/schema";
import { useLanguage } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { formatDateTime } from "../../ui/format";
import { OverviewPanel } from "./OverviewPanel";

type RecentConnection = components["schemas"]["RecentConnection"];

// The connections listed; the rest are counted under them.
const shownConnections = 3;

// RecentConnectionsPanel lists the connections used last, each opening a session to it.
export function RecentConnectionsPanel({
  recent,
  onOpenConnection,
  onShowConnections,
}: {
  recent: readonly RecentConnection[];
  onOpenConnection: (alias: string) => void;
  onShowConnections: () => void;
}) {
  const { t, locale } = useLanguage();
  const more = recent.length - shownConnections;
  return (
    <OverviewPanel
      icon="history"
      title={t("terminal.overview.recent")}
      action={<button type="button" className="text-[12.5px] font-normal text-accent hover:underline" onClick={onShowConnections}>{t("terminal.overview.showAllConnections")}</button>}
      {...(more > 0 ? { footer: <span>{t("terminal.overview.moreRecent", { count: more })}</span> } : {})}
    >
      <ul>
        {recent.slice(0, shownConnections).map((connection) => (
          <li key={connection.alias} className="border-t border-line first:border-t-0">
            <button type="button" onClick={() => onOpenConnection(connection.alias)}
              className="flex w-full items-center gap-2.5 py-[7px] text-left hover:bg-hover">
              <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-[color-mix(in_srgb,var(--ui-ink)_7%,transparent)] text-ink-muted">
                <Icon name="connections" className="size-[15px]" />
              </span>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-[13.5px] font-semibold text-ink">{connection.alias}</span>
                <span className="truncate text-xs text-ink-muted">{connection.user === "" ? connection.hostName : `${connection.user}@${connection.hostName}`} · {formatDateTime(connection.lastConnectedAt, locale)}</span>
              </span>
              <Icon name="chevronRight" className="size-4 shrink-0 text-ink-faint" />
            </button>
          </li>
        ))}
      </ul>
    </OverviewPanel>
  );
}

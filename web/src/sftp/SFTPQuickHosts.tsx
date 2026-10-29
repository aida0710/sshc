import { useEffect, useState } from "react";
import { recentConnectionsApi, type RecentConnection } from "../api/recentConnections";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { localHostAlias } from "./localHost";

// How many recent connections the empty pane offers besides this machine. More are in the picker.
const quickRecentCount = 4;

// SFTPQuickHosts lets an empty pane open this machine or a recently used connection with one press, without
// going through the picker first. Only connections that still exist are offered.
export function SFTPQuickHosts({ aliases, onChoose, loadRecent = () => recentConnectionsApi.recentConnections() }: {
  aliases: readonly string[];
  onChoose: (alias: string) => void;
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
}) {
  const t = useTranslate();
  const [recent, setRecent] = useState<string[]>([]);
  useEffect(() => {
    let active = true;
    void loadRecent().then((loaded) => {
      if (active) setRecent(loaded.connections.map((connection) => connection.alias));
    }).catch(() => undefined);
    return () => { active = false; };
  }, [loadRecent]);
  const known = new Set(aliases);
  const offered = recent.filter((alias) => known.has(alias)).slice(0, quickRecentCount);
  const choice = (alias: string, label: string, icon: "home" | "terminal") => (
    <li key={alias}>
      <button
        type="button"
        onClick={() => onChoose(alias)}
        className="flex w-full min-w-0 items-center gap-2.5 rounded-lg px-3 py-2 text-left text-[13px] text-ink transition-colors hover:bg-hover"
      >
        <Icon name={icon} className="size-4 shrink-0 text-accent" />
        <span className="truncate">{label}</span>
      </button>
    </li>
  );
  return (
    <ul aria-label={t("sftp.quickHosts")} className="mt-3 flex w-full max-w-[280px] flex-col gap-0.5 text-left">
      {choice(localHostAlias, t("sftp.local.connection"), "home")}
      {offered.map((alias) => choice(alias, alias, "terminal"))}
    </ul>
  );
}

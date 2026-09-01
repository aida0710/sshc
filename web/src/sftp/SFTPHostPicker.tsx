import { useEffect, useMemo, useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import { integrationsApi, type RecentConnection } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";

type HostChoice = { alias: string; group: string; hostName: string; user: string };
const loadDefaultRecent = () => integrationsApi.recentConnections();
const noHosts: HostEntry[] = [];

function hostChoices(aliases: string[], hosts: HostEntry[]): HostChoice[] {
  const byAlias = new Map(hosts.map((host) => [host.identity.alias, host]));
  return aliases.map((alias) => {
    const host = byAlias.get(alias);
    return { alias, group: host?.group ?? "", hostName: host?.hostName ?? "", user: host?.user ?? "" };
  });
}

export function SFTPHostPicker({
  aliases,
  hosts = noHosts,
  value,
  disabled = false,
  loadRecent = loadDefaultRecent,
  onChange,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  value: string;
  disabled?: boolean;
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
  onChange: (alias: string) => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [view, setView] = useState<"recent" | "groups">("groups");
  const [recent, setRecent] = useState<RecentConnection[]>([]);
  const search = useRef<HTMLInputElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const available = useMemo(() => hostChoices(aliases, hosts), [aliases, hosts]);
  const byAlias = useMemo(() => new Map(available.map((host) => [host.alias, host])), [available]);
  const normalized = query.trim().toLocaleLowerCase();
  const matches = normalized === "" ? available : available.filter((host) =>
    [host.alias, host.group, host.hostName, host.user].some((field) => field.toLocaleLowerCase().includes(normalized)),
  );
  const recentChoices = recent.flatMap((item) => {
    const host = byAlias.get(item.alias);
    return host === undefined ? [] : [{ ...host, lastConnectedAt: item.lastConnectedAt }];
  });
  const grouped = useMemo(() => {
    const result = new Map<string, HostChoice[]>();
    for (const host of matches) result.set(host.group, [...(result.get(host.group) ?? []), host]);
    return result;
  }, [matches]);

  useEffect(() => {
    if (!open) return;
    let active = true;
    void loadRecent().then((loaded) => {
      if (!active) return;
      setRecent(loaded.connections);
      if (loaded.connections.some((item) => byAlias.has(item.alias))) setView("recent");
    }).catch(() => undefined);
    return () => { active = false; };
  }, [open, loadRecent, byAlias]);

  function choose(alias: string) {
    onChange(alias);
    setOpen(false);
    setQuery("");
  }

  const row = (host: HostChoice, detail: string) => (
    <button key={host.alias} type="button" onClick={() => choose(host.alias)} className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-left hover:bg-select-fill focus:bg-select-fill focus:outline-none ${host.alias === value ? "bg-select-fill" : ""}`}>
      <Icon name="terminal" className="size-4 shrink-0 text-accent" />
      <span className="min-w-0 grow">
        <span className="block truncate font-medium text-ink">{host.alias}</span>
        <span className="block truncate text-xs text-ink-muted">{detail || host.hostName || t("sftp.hostNoDetails")}</span>
      </span>
      {host.alias === value ? <span className="text-xs text-accent">{t("sftp.hostCurrent")}</span> : null}
    </button>
  );

  return (
    <>
      <button ref={trigger} type="button" aria-label={t("sftp.host")} data-value={value} disabled={disabled || aliases.length === 0} onClick={() => setOpen(true)} className="flex min-h-9 min-w-36 items-center justify-between gap-2 rounded-md border border-control-line bg-control px-3 py-1.5 text-left text-sm disabled:text-ink-faint">
        <span className="truncate">{value || t(aliases.length === 0 ? "sftp.noHosts" : "sftp.chooseHost")}</span>
        <Icon name="chevronRight" className="size-3 rotate-90 text-ink-muted" />
      </button>
      <ModalShell open={open} labelledBy="sftp-host-picker-heading" onDismiss={() => setOpen(false)} closeOnOutside initialFocusRef={search} returnFocusRef={trigger} placement="palette" panelClassName="flex max-h-[76vh] w-full max-w-xl flex-col overflow-hidden rounded-xl">
        <div className="border-b border-line p-3">
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 id="sftp-host-picker-heading" className="font-semibold">{t("sftp.chooseHostHeading")}</h2>
            <button type="button" aria-label={t("sftp.closeHostPicker")} onClick={() => setOpen(false)} className="flex size-8 items-center justify-center rounded text-ink-muted hover:bg-select-fill">×</button>
          </div>
          <label className="relative block">
            <span className="sr-only">{t("sftp.searchHosts")}</span>
            <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-muted" />
            <input ref={search} type="search" aria-label={t("sftp.searchHosts")} value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t("sftp.searchHostsPlaceholder")} className="w-full rounded-md border border-control-line bg-control py-2 pl-9 pr-3 text-sm" />
          </label>
          {normalized === "" ? <div className="mt-3 flex gap-1 rounded-md bg-toolbar p-1" role="tablist" aria-label={t("sftp.hostViews")}>
            <button type="button" role="tab" aria-selected={view === "recent"} disabled={recentChoices.length === 0} onClick={() => setView("recent")} className={`grow rounded px-3 py-1.5 text-sm disabled:text-ink-faint ${view === "recent" ? "bg-card shadow-sm" : "text-ink-muted"}`}>{t("sftp.recentHosts")}</button>
            <button type="button" role="tab" aria-selected={view === "groups"} onClick={() => setView("groups")} className={`grow rounded px-3 py-1.5 text-sm ${view === "groups" ? "bg-card shadow-sm" : "text-ink-muted"}`}>{t("sftp.hostGroups")}</button>
          </div> : null}
        </div>
        <div className="min-h-0 overflow-y-auto p-2">
          {normalized !== "" ? (
            matches.length === 0 ? <p className="p-4 text-center text-sm text-ink-muted">{t("sftp.noHostMatches")}</p> : matches.map((host) => row(host, host.group || host.hostName))
          ) : view === "recent" && recentChoices.length > 0 ? (
            <section aria-labelledby="sftp-recent-hosts-heading">
              <h3 id="sftp-recent-hosts-heading" className="px-3 py-2 text-xs font-medium uppercase tracking-wide text-ink-muted">{t("sftp.recentHosts")}</h3>
              {recentChoices.map((host) => row(host, t("sftp.lastConnected", { at: new Date(host.lastConnectedAt).toLocaleString() })))}
            </section>
          ) : (
            [...grouped.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([group, groupHosts]) => (
              <section key={group || "ungrouped"} aria-label={group || t("sftp.ungroupedHosts")} className="mb-2">
                <h3 className="px-3 py-2 text-xs font-medium uppercase tracking-wide text-ink-muted">{group || t("sftp.ungroupedHosts")}</h3>
                {groupHosts.map((host) => row(host, host.hostName))}
              </section>
            ))
          )}
        </div>
      </ModalShell>
    </>
  );
}

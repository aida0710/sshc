import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import type { HostEntry } from "../api/config";
import { recentConnectionsApi, type RecentConnection } from "../api/recentConnections";
import { useTranslate } from "../i18n/context";
import { localHostAlias } from "../sftp/localHost";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";
import { activateTabFromKeyboard } from "../ui/tabKeyboard";

type HostChoice = { alias: string; group: string; hostName: string; user: string };

// A LocalChoice is pinned above the SSH hosts. SFTP offers the engine's own
// file system; the console list offers local shells, one per profile.
export type LocalChoice = { id: string; label: string; detail: string };

const loadDefaultRecent = () => recentConnectionsApi.recentConnections();
const noHosts: HostEntry[] = [];
const noLocal: LocalChoice[] = [];

function hostChoices(aliases: string[], hosts: HostEntry[]): HostChoice[] {
  const byAlias = new Map(hosts.map((host) => [host.identity.alias, host]));
  return aliases.map((alias) => {
    const host = byAlias.get(alias);
    return { alias, group: host?.group ?? "", hostName: host?.hostName ?? "", user: host?.user ?? "" };
  });
}

// HostPickerDialog is the one place a connection is chosen from: search,
// recently used hosts and the declared groups. SFTP and the console list
// share it so both destinations look and behave the same.
export function HostPickerDialog({
  open,
  heading,
  aliases,
  hosts = noHosts,
  value = "",
  local = noLocal,
  loadRecent = loadDefaultRecent,
  initialFocus = "search",
  returnFocusRef,
  onChoose,
  onChooseLocal,
  onClose,
}: {
  open: boolean;
  heading: string;
  aliases: string[];
  hosts?: HostEntry[];
  value?: string;
  local?: LocalChoice[];
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
  initialFocus?: "search" | "close";
  returnFocusRef?: RefObject<HTMLElement | null>;
  onChoose: (alias: string) => void;
  onChooseLocal?: (id: string) => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  const [query, setQuery] = useState("");
  const [view, setView] = useState<"recent" | "groups">("groups");
  const [recent, setRecent] = useState<RecentConnection[]>([]);
  const search = useRef<HTMLInputElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const available = useMemo(() => hostChoices(aliases, hosts), [aliases, hosts]);
  const byAlias = useMemo(() => new Map(available.map((host) => [host.alias, host])), [available]);
  const normalized = query.trim().toLocaleLowerCase();
  const localMatches = local.filter((choice) => normalized === "" ||
    [choice.label, choice.detail, "local"].some((field) => field.toLocaleLowerCase().includes(normalized)));
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
  const views = recentChoices.length === 0 ? (["groups"] as const) : (["recent", "groups"] as const);

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

  function close() {
    onClose();
    setQuery("");
  }

  const row = (key: string, name: string, detail: string, icon: "home" | "terminal", current: boolean, choose: () => void, label?: string) => (
    <button key={key} type="button" onClick={() => { choose(); setQuery(""); }}
      aria-label={label}
      className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-left hover:bg-select-fill focus:bg-select-fill focus:outline-none ${current ? "bg-select-fill" : ""}`}>
      <Icon name={icon} className="size-4 shrink-0 text-accent" />
      <span className="min-w-0 grow">
        <span className="block truncate font-medium text-ink">{name}</span>
        <span className="block truncate text-xs text-ink-muted">{detail || t("sftp.hostNoDetails")}</span>
      </span>
      {current ? <span className="text-xs text-accent">{t("sftp.hostCurrent")}</span> : null}
    </button>
  );
  const hostRow = (host: HostChoice, detail: string) =>
    row(host.alias, host.alias, detail || host.hostName, "terminal", host.alias === value, () => onChoose(host.alias));

  return (
    <ModalShell open={open} labelledBy="host-picker-heading" onDismiss={close} closeOnOutside initialFocusRef={initialFocus === "close" ? closeButton : search} {...(returnFocusRef === undefined ? {} : { returnFocusRef })} placement="palette" panelClassName="flex max-h-[76vh] w-full max-w-xl flex-col overflow-hidden rounded-xl">
      <div className="border-b border-line p-3">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 id="host-picker-heading" className="font-semibold">{heading}</h2>
          <button ref={closeButton} type="button" aria-label={t("sftp.closeHostPicker")} onClick={close} className="flex size-8 items-center justify-center rounded text-ink-muted hover:bg-select-fill">×</button>
        </div>
        <label className="relative block">
          <span className="sr-only">{t("sftp.searchHosts")}</span>
          <Icon name="search" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-muted" />
          <input ref={search} type="search" aria-label={t("sftp.searchHosts")} value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t("sftp.searchHostsPlaceholder")} className="w-full rounded-md border border-control-line bg-control py-2 pl-9 pr-3 text-sm" />
        </label>
        {normalized === "" ? <div className="mt-3 flex gap-1 rounded-md bg-toolbar p-1" role="tablist" aria-label={t("sftp.hostViews")}>
          <button type="button" role="tab" aria-selected={view === "recent"} tabIndex={view === "recent" ? 0 : -1} disabled={recentChoices.length === 0} onClick={() => setView("recent")} onKeyDown={(event) => activateTabFromKeyboard(event, 0, views, setView)} className={`grow rounded px-3 py-1.5 text-sm disabled:text-ink-faint ${view === "recent" ? "bg-card shadow-sm" : "text-ink-muted"}`}>{t("sftp.recentHosts")}</button>
          <button type="button" role="tab" aria-selected={view === "groups"} tabIndex={view === "groups" ? 0 : -1} onClick={() => setView("groups")} onKeyDown={(event) => activateTabFromKeyboard(event, views.length - 1, views, setView)} className={`grow rounded px-3 py-1.5 text-sm ${view === "groups" ? "bg-card shadow-sm" : "text-ink-muted"}`}>{t("sftp.hostGroups")}</button>
        </div> : null}
      </div>
      <div className="min-h-0 overflow-y-auto p-2">
        {localMatches.length > 0 ? <div className="mb-2 border-b border-line pb-2">
          {localMatches.map((choice) => row(`local:${choice.id}`, choice.label, choice.detail, "home",
            value === localHostAlias && choice.id === localMatches[0]?.id, () => onChooseLocal?.(choice.id), `${choice.label}, ${choice.detail}`))}
        </div> : null}
        {normalized !== "" ? (
          matches.length === 0 && localMatches.length === 0 ? <p className="p-4 text-center text-sm text-ink-muted">{t("sftp.noHostMatches")}</p> : matches.map((host) => hostRow(host, host.group || host.hostName))
        ) : view === "recent" && recentChoices.length > 0 ? (
          <section aria-labelledby="host-picker-recent-heading">
            <h3 id="host-picker-recent-heading" className="px-3 py-2 text-xs font-medium uppercase tracking-wide text-ink-muted">{t("sftp.recentHosts")}</h3>
            {recentChoices.map((host) => hostRow(host, t("sftp.lastConnected", { at: new Date(host.lastConnectedAt).toLocaleString() })))}
          </section>
        ) : (
          [...grouped.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([group, groupHosts]) => (
            <section key={group || "ungrouped"} aria-label={group || t("sftp.ungroupedHosts")} className="mb-2">
              <h3 className="px-3 py-2 text-xs font-medium uppercase tracking-wide text-ink-muted">{group || t("sftp.ungroupedHosts")}</h3>
              {groupHosts.map((host) => hostRow(host, host.hostName))}
            </section>
          ))
        )}
      </div>
    </ModalShell>
  );
}

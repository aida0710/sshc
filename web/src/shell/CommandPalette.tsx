import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import type { FileNode, HostEntry, HostIdentity } from "../api/config";
import { snippetsApi, type Snippet } from "../snippets/api";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import { Icon } from "../ui/icons";
import type { TerminalSession } from "../api/integrations";
import { agentStatusLabel, terminalDisplayTitle } from "../terminal/agentPresentation";
import type { AgentUnreadBySession } from "../terminal/agentNotifications";
import { ModalShell } from "../ui/ModalShell";

type PaletteItem = {
  id: string;
  kind: "session" | "host" | "file" | "snippet" | "setting";
  label: string;
  detail: string;
  search: string;
  action: () => void | Promise<void>;
  host?: HostIdentity;
};

const searchableSections: Section[] = [
  "Connections", "Config", "Groups", "Keys", "Known Hosts", "Remote Keys",
  "Diagnostics", "Secrets", "Snippets", "Settings", "Sync", "History",
];

function matches(item: PaletteItem, query: string): boolean {
  const tokens = query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return true;
  const haystack = `${item.label} ${item.detail} ${item.search}`.toLocaleLowerCase();
  return tokens.every((token) => haystack.includes(token));
}

export function CommandPalette({
  open,
  hosts,
  files,
  sessions,
  unreadBySession,
  sectionLabels,
  onClose,
  onConnect,
  onOpenHostSettings,
  onOpenFile,
  onNavigate,
  onOpenSnippet,
  onOpenSession,
  returnFocusRef,
}: {
  open: boolean;
  hosts: HostEntry[];
  files: FileNode[];
  sessions: TerminalSession[];
  unreadBySession: AgentUnreadBySession;
  sectionLabels: Record<Section, MessageKey>;
  onClose: () => void;
  onConnect: (alias: string) => Promise<void> | void;
  onOpenHostSettings: (identity: HostIdentity) => void;
  onOpenFile: (path: string) => void;
  onNavigate: (section: Section) => void;
  onOpenSnippet: (id: string) => void;
  onOpenSession: (id: string) => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const input = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setSelected(0);
    window.requestAnimationFrame(() => input.current?.focus());
    let active = true;
    setLoading(true);
    void snippetsApi.library()
      .then((library) => {
        if (active) setSnippets(library.snippets);
      })
      .catch(() => {
        if (active) setSnippets([]);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [open]);

  const items = useMemo<PaletteItem[]>(() => [
    ...sessions.filter((session) => session.exited === undefined).map((session) => {
      const unread = unreadBySession.get(session.id);
      const status = session.agent === undefined ? t("terminal.connected") : agentStatusLabel(t, session);
      const destination = session.kind === "ssh" ? session.alias ?? "" : t("terminal.localhost");
      return {
        id: `session:${session.id}`,
        kind: "session" as const,
        label: terminalDisplayTitle(session),
        detail: t("terminal.rowDetail", { status, destination }),
        search: `session terminal console pane セッション ターミナル ${unread === "attention" ? "@attention attention input 入力待ち" : ""} ${unread === "completed" ? "@completed completed unread 完了 未読" : ""}`,
        action: () => onOpenSession(session.id),
      };
    }),
    ...hosts.map((host) => ({
      id: `host:${host.identity.path}:${host.identity.alias}`,
      kind: "host" as const,
      label: t("palette.connectHost", { alias: host.identity.alias }),
      detail: host.file.path ?? host.file.absolute,
      search: `${host.identity.alias} ${host.identity.path} host connect ssh 接続 ホスト`,
      action: () => onConnect(host.identity.alias),
      host: host.identity,
    })),
    ...files.map((node) => ({
      id: `file:${node.file.path ?? node.file.absolute}`,
      kind: "file" as const,
      label: node.file.path ?? node.file.absolute,
      detail: node.file.absolute,
      search: `file config ssh 設定ファイル ${node.file.path ?? ""}`,
      action: () => onOpenFile(node.file.path ?? node.file.absolute),
    })),
    ...snippets.map((snippet) => ({
      id: `snippet:${snippet.id}`,
      kind: "snippet" as const,
      label: snippet.name,
      detail: snippet.command,
      search: `snippet snippets スニペット ${snippet.description ?? ""}`,
      action: () => onOpenSnippet(snippet.id),
    })),
    ...searchableSections.map((section) => ({
      id: `setting:${section}`,
      kind: "setting" as const,
      label: t(sectionLabels[section]),
      detail: t("palette.openSection"),
      search: `${section} settings setting 設定`,
      action: () => onNavigate(section),
    })),
  ], [files, hosts, onConnect, onNavigate, onOpenFile, onOpenSession, onOpenSnippet, sectionLabels, sessions, snippets, t, unreadBySession]);

  const visible = useMemo(() => items.filter((item) => matches(item, query)).slice(0, 40), [items, query]);

  useEffect(() => {
    if (selected >= visible.length) setSelected(Math.max(0, visible.length - 1));
  }, [selected, visible.length]);

  if (!open) return null;

  async function choose(item: PaletteItem) {
    onClose();
    await item.action();
  }

  function useKeyboard(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelected((current) => visible.length === 0 ? 0 : (current + 1) % visible.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelected((current) => visible.length === 0 ? 0 : (current - 1 + visible.length) % visible.length);
    } else if (event.key === "Enter" && visible[selected] !== undefined) {
      event.preventDefault();
      void choose(visible[selected]);
    }
  }

  return (
    <ModalShell
      labelledBy="command-palette-heading"
      onDismiss={onClose}
      initialFocusRef={input}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      placement="palette"
      zIndexClassName="z-[70]"
      panelClassName="flex max-h-[72vh] w-full max-w-2xl flex-col overflow-hidden rounded-lg"
    >
        <h2 id="command-palette-heading" className="sr-only">{t("palette.heading")}</h2>
        <label className="flex h-12 shrink-0 items-center gap-2 border-b border-line px-3">
          <Icon name="search" className="h-4 w-4 text-ink-muted" />
          <span className="sr-only">{t("palette.heading")}</span>
          <input
            ref={input}
            type="search"
            aria-label={t("palette.heading")}
            aria-controls="command-palette-results"
            aria-activedescendant={visible[selected] === undefined ? undefined : `command-palette-option-${selected}`}
            value={query}
            onChange={(event) => { setQuery(event.target.value); setSelected(0); }}
            onKeyDown={useKeyboard}
            placeholder={t("palette.placeholder")}
            className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
          />
          <kbd className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink-muted">Esc</kbd>
        </label>
        <div id="command-palette-results" role="listbox" aria-label={t("palette.results")} className="min-h-0 overflow-y-auto p-1.5">
          {visible.map((item, index) => (
            <div key={item.id} className="relative">
              <button
                id={`command-palette-option-${index}`}
                type="button"
                role="option"
                aria-selected={selected === index}
                onMouseEnter={() => setSelected(index)}
                onClick={() => void choose(item)}
                className={`grid w-full grid-cols-[4.5rem_minmax(0,1fr)] items-center gap-x-3 rounded px-2.5 py-2 text-left text-sm ${item.host === undefined ? "" : "pr-12"} ${selected === index ? "bg-select-fill" : "hover:bg-select-fill"}`}
              >
                <span className="row-span-2 font-mono text-[10px] uppercase tracking-wide text-ink-faint">{t(`palette.kind.${item.kind}` as MessageKey)}</span>
                <span className="truncate font-medium text-ink">{item.label}</span>
                <span className="truncate font-mono text-[11px] text-ink-muted">{item.detail}</span>
              </button>
              {item.host === undefined ? null : (
                <button
                  type="button"
                  aria-label={t("palette.openHostSettings", { alias: item.host.alias })}
                  title={t("home.openConnectionSettings")}
                  onMouseEnter={() => setSelected(index)}
                  onClick={() => {
                    const host = item.host;
                    if (host === undefined) return;
                    onClose();
                    onOpenHostSettings(host);
                  }}
                  className="absolute right-1.5 top-1/2 flex size-9 -translate-y-1/2 items-center justify-center rounded text-ink-muted hover:bg-surface hover:text-ink"
                >
                  <Icon name="moreHorizontal" className="size-4" />
                </button>
              )}
            </div>
          ))}
          {visible.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-ink-muted">{loading ? t("palette.loading") : t("palette.empty")}</p>
          ) : null}
        </div>
        <p className="shrink-0 border-t border-line px-3 py-2 text-[11px] text-ink-faint">{t("palette.hint")}</p>
    </ModalShell>
  );
}

export { matches };

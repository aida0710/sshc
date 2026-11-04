import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { shownFilePath } from "../android/shownFilePath";
import type { FileNode, HostEntry, HostIdentity } from "../api/config";
import { snippetsApi, type Snippet } from "../snippets/api";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import type { Section } from "../routing/sectionRoute";
import { shortcutLabel } from "../keyconfig/shortcutLabel";
import { Icon } from "../ui/icons";
import type { TerminalSession } from "../api/terminalSessions";
import { terminalDisplayTitle, terminalSubtitle } from "../terminal/terminalPresentation";
import type { UnreadSessions } from "../terminal/terminalNotifications";
import { ModalShell } from "../ui/ModalShell";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { PanelState } from "../ui/PanelState";
import { isImeComposing } from "../ui/imeComposition";
import { hasOnScreenKeyboard, literalTextEntry } from "../ui/softKeyboard";
import { parseDirectDestination } from "../connections/directDestination";
import type { DirectDestinationActions } from "./useDirectDestinationActions";
import { searchPalette } from "./paletteSearch";

// A command is something the palette performs, rather than somewhere it goes.
export type PaletteCommand = {
  id: string;
  label: string;
  // detail is a second line, or empty for none.
  detail: string;
  search: string;
  // kind "setting" lists it among the settings; it is an action otherwise.
  kind?: "setting";
  // keys are the shortcuts assigned to it, the first of them shown beside it.
  keys?: readonly string[];
  run: () => void | Promise<void>;
};

type PaletteItem = {
  id: string;
  kind: "command" | "session" | "host" | "file" | "snippet" | "setting";
  label: string;
  detail: string;
  search: string;
  action: () => void | Promise<void>;
  host?: HostIdentity;
  keys?: readonly string[] | undefined;
};

const kindLabelKeys: Record<PaletteItem["kind"], MessageKey> = {
  command: "palette.kind.command",
  session: "palette.kind.session",
  host: "palette.kind.host",
  file: "palette.kind.file",
  snippet: "palette.kind.snippet",
  setting: "palette.kind.setting",
};

// Enough for every command and page with some connections and sessions before them, so all of them can be reached
// by scrolling; typing narrows it down when there are more.
const maximumShownResults = 100;

const searchableSections: Section[] = [
  "Connections", "Config", "Keys", "Known Hosts", "Remote Keys",
  "Diagnostics", "Passwords", "Key Passphrases", "OTP", "Snippets", "Settings", "Sync", "VPN", "History",
];

function paletteItemOf(command: PaletteCommand): PaletteItem {
  return {
    id: `command:${command.id}`,
    kind: command.kind ?? "command",
    label: command.label,
    detail: command.detail,
    search: `${command.search} ${command.kind === "setting" ? "settings setting 設定" : "command action 実行 コマンド 操作"}`,
    action: command.run,
    keys: command.keys,
  };
}

export function CommandPalette({
  open,
  commands = [],
  laterCommands = [],
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
  direct,
  returnFocusRef,
}: {
  open: boolean;
  // commands lead the list; laterCommands follow everything else until something is typed, when they are found
  // among the commands. The palette is mostly used to start a connection, which they would otherwise push down.
  commands?: PaletteCommand[];
  laterCommands?: PaletteCommand[];
  hosts: HostEntry[];
  files: FileNode[];
  sessions: TerminalSession[];
  unreadBySession: UnreadSessions;
  sectionLabels: Record<Section, MessageKey>;
  onClose: () => void;
  onConnect: (alias: string) => Promise<void> | void;
  onOpenHostSettings: (identity: HostIdentity) => void;
  onOpenFile: (path: string) => void;
  onNavigate: (section: Section) => void;
  onOpenSnippet: (id: string) => void;
  onOpenSession: (id: string) => void;
  // direct lets the search take user@host[:port] as well as find something.
  direct?: DirectDestinationActions | undefined;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const input = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // Forgotten on closing, not on opening: the next opening would first draw, and take a typed character as, the
    // last query.
    if (!open) {
      setQuery("");
      setSelected(0);
      return;
    }
    if (!hasOnScreenKeyboard()) window.requestAnimationFrame(() => input.current?.focus());
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
    // Commands lead: typing "new" should reach the action before it reaches a
    // host whose name happens to contain the word.
    ...commands.map(paletteItemOf),
    // Hosts come before open sessions: the palette is mostly used to start a
    // connection, and a session to a host is reachable from the session list.
    ...hosts.map((host) => ({
      id: `host:${host.identity.path}:${host.identity.alias}`,
      kind: "host" as const,
      label: t("palette.connectHost", { alias: host.identity.alias }),
      detail: host.file.path ?? host.file.absolute,
      search: `${host.identity.alias} ${host.identity.path} host connect ssh 接続 ホスト`,
      action: () => onConnect(host.identity.alias),
      host: host.identity,
    })),
    ...sessions.filter((session) => session.exited === undefined).map((session) => {
      const unread = unreadBySession.has(session.id);
      const status = t("terminal.connected");
      const destination = terminalSubtitle(session, t);
      return {
        id: `session:${session.id}`,
        kind: "session" as const,
        label: terminalDisplayTitle(session),
        detail: t("terminal.rowDetail", { status, destination }),
        search: `session terminal console pane セッション ターミナル ${unread ? "@unread unread notification 未読 通知" : ""}`,
        action: () => onOpenSession(session.id),
      };
    }),
    ...files.map((node) => ({
      id: `file:${node.file.path ?? node.file.absolute}`,
      kind: "file" as const,
      label: node.file.path ?? node.file.absolute,
      detail: shownFilePath(node.file.absolute),
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
    ...laterCommands.map(paletteItemOf),
  ], [commands, laterCommands, files, hosts, onConnect, onNavigate, onOpenFile, onOpenSession, onOpenSnippet, sectionLabels, sessions, snippets, t, unreadBySession]);

  // A destination leads the results whatever else it finds: it is only offered when it was typed whole.
  const directItems = useMemo<PaletteItem[]>(() => {
    const destination = direct === undefined ? null : parseDirectDestination(query);
    if (direct === undefined || destination === null) return [];
    const { saveAndConnect } = direct;
    const connect: PaletteItem = {
      id: "direct:connect", kind: "host", search: "",
      label: t("directDestination.connect"),
      detail: t("directDestination.connectDetail"),
      action: () => direct.connect(destination),
    };
    if (saveAndConnect === undefined) return [connect];
    return [connect, {
      id: "direct:save", kind: "host", search: "",
      label: t("directDestination.saveAndConnect"),
      detail: t("directDestination.saveAndConnectDetail", { alias: direct.aliasFor(destination) }),
      action: () => saveAndConnect(destination),
    }];
  }, [direct, query, t]);
  const visible = useMemo(() => [
    ...directItems,
    ...searchPalette(items.map((item) => ({ ...item, name: item.host?.alias ?? item.label })), query),
  ].slice(0, maximumShownResults), [directItems, items, query]);

  useEffect(() => {
    if (selected >= visible.length) setSelected(Math.max(0, visible.length - 1));
  }, [selected, visible.length]);

  if (!open) return null;

  async function choose(item: PaletteItem) {
    onClose();
    await item.action();
  }

  // Arrow keys move through the results; Enter runs the selected one.
  function navigateResultsByKey(event: KeyboardEvent<HTMLInputElement>) {
    if (isImeComposing(event)) return;
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
      closeOnOutside
      initialFocusRef={input}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      placement="palette"
      zIndexClassName="z-[70]"
      panelClassName="flex max-h-[72vh] w-full max-w-2xl flex-col overflow-hidden rounded-lg"
    >
        <h2 id="command-palette-heading" className="sr-only">{t("palette.heading")}</h2>
        <div className="flex h-12 shrink-0 items-center gap-2 border-b border-line pl-3 pr-1 desktop-layout:pr-3">
          <label className="flex h-full min-w-0 flex-1 items-center gap-2">
            <Icon name="search" className="h-4 w-4 text-ink-muted" />
            <span className="sr-only">{t("palette.heading")}</span>
            <input {...literalTextEntry}
              ref={input}
              type="search"
              aria-label={t("palette.heading")}
              aria-controls="command-palette-results"
              aria-activedescendant={visible[selected] === undefined ? undefined : `command-palette-option-${selected}`}
              value={query}
              onChange={(event) => { setQuery(event.target.value); setSelected(0); }}
              onKeyDown={navigateResultsByKey}
              placeholder={t("palette.placeholder")}
              // The row is the field, and the caret says the focus is in it; the ring of other fields would cross the row's edges.
              className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none! placeholder:text-ink-faint"
            />
          </label>
          {compact ? (
            <button type="button" aria-label={t("palette.close")} title={t("palette.close")} onClick={onClose} className="flex shrink-0 items-center justify-center rounded text-ink-muted hover:bg-surface hover:text-ink">
              <Icon name="close" className="size-4" />
            </button>
          ) : <kbd className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink-muted">Esc</kbd>}
        </div>
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
                className={`grid w-full grid-cols-[4.5rem_minmax(0,1fr)_auto] items-center gap-x-3 rounded px-2.5 py-2 text-left text-sm ${item.host === undefined ? "" : "pr-12"} ${selected === index ? "bg-select-fill" : "hover:bg-hover"}`}
              >
                <span className="row-span-2 font-mono text-[10px] uppercase tracking-wide text-ink-faint">{t(kindLabelKeys[item.kind])}</span>
                <span title={item.label} className="col-start-2 truncate font-medium text-ink">{item.label}</span>
                {item.detail === "" ? null : <span title={item.detail} className="col-start-2 truncate font-mono text-[11px] text-ink-muted">{item.detail}</span>}
                {/* A phone has no keys to press, and its width goes to the name. */}
                {item.keys?.[0] === undefined ? null : (
                  <kbd title={item.keys.map(shortcutLabel).join(" / ")} className="col-start-3 row-span-2 row-start-1 hidden border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink-muted desktop-layout:block">{shortcutLabel(item.keys[0])}</kbd>
                )}
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
                  <Icon name="settings" className="size-4" />
                </button>
              )}
            </div>
          ))}
          {visible.length === 0 ? (
            <PanelState tone={loading ? "loading" : "empty"} title={loading ? t("palette.loading") : t("palette.empty")} />
          ) : null}
        </div>
        <p className="shrink-0 border-t border-line px-3 py-2 text-[11px] text-ink-faint">{t(compact ? "palette.hintCompact" : "palette.hint")}</p>
    </ModalShell>
  );
}


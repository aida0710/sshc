import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { fontStack } from "./fonts";
import { defaultTint } from "./appearance";
import { useBackgroundImage } from "./backgroundImage";
import { clipboard } from "../ui/clipboard";
import { attachImeKeys } from "./imeKeys";
import { attachSelectionOverlay, selectionHeldIn } from "./selectionOverlay";
import { prefersNativeSelection } from "./nativeSelection";
import { cellHeight } from "./metrics";
import { newTouchScroll } from "./touchScroll";
import { KeyBar, applyModifiers, encodeKey, type Modifiers } from "./KeyBar";
import { openStream, type TerminalStream } from "./stream";
import { attachTerminalClipboard, type TerminalClipboardSettings } from "./clipboard";
import { sftpApi } from "../sftp/api";
import { absolutePathDraft, findBufferMatches, frequentCommandSuggestions, updateCommandDraft } from "./productivity";
import { terminalProblemKey } from "./sessions";

type TerminalViewProps = {
  session: TerminalSession;
  api?: Pick<IntegrationsApi, "terminalStreamTicket">;
  onExit?: () => void;
  onReconnect?: () => Promise<boolean>;
  copyOnSelect?: boolean;
  fontSize?: number;
  rightClickPaste?: boolean;
  palette?: string;
  background?: string;
  tint?: number;
  font?: string;
};

type Link =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  | { phase: "stopped"; gone: boolean };

const backoff = [1, 2, 4, 8, 15];

const settled = 10_000;

type Completion = { kind: "command" | "path"; value: string; prefix: string };

export function TerminalView({
  session,
  api = integrationsApi,
  onExit,
  onReconnect,
  copyOnSelect = true,
  fontSize,
  rightClickPaste = true,
  palette,
  font,
  background,
  tint,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const host = useRef<HTMLDivElement>(null);
  const backgroundURL = useBackgroundImage(background ?? "");
  const hasBackground = backgroundURL !== "";
  const refit = useRef<(() => void) | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const clipboardSettings = useRef<TerminalClipboardSettings>({ copyOnSelect, rightClickPaste });
  clipboardSettings.current = { copyOnSelect, rightClickPaste };
  const [problem, setProblem] = useState("");
  const [manualReconnectBusy, setManualReconnectBusy] = useState(false);
  const [link, setLink] = useState<Link>({ phase: "connecting", attempt: 1 });
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResult, setSearchResult] = useState({ index: -1, total: 0 });
  const searchQueryRef = useRef("");
  searchQueryRef.current = searchQuery;
  const searchStep = useRef<(direction: 1 | -1) => void>(() => {});
  const [commandDraft, setCommandDraft] = useState("");
  const commandDraftRef = useRef("");
  const [commandHistory, setCommandHistory] = useState<string[]>([]);
  const [pathSuggestions, setPathSuggestions] = useState<string[]>([]);
  const control = useRef<{ now: () => void; stop: () => void }>({ now: () => {}, stop: () => {} });

  const [modifiers, setModifiers] = useState<Modifiers>({ ctrl: false, alt: false });
  const armed = useRef<Modifiers>(modifiers);
  armed.current = modifiers;
  const send = useRef<(label: string) => void>(() => {});
  const acceptCompletion = useRef<(completion: Completion) => void>(() => {});
  const commandSuggestions = useMemo(() => frequentCommandSuggestions(commandHistory, commandDraft), [commandDraft, commandHistory]);
  const completionItems = useMemo<Completion[]>(() => [
    ...commandSuggestions.map((value) => ({ kind: "command" as const, value, prefix: commandDraft.trimStart() })),
    ...pathSuggestions.map((value) => ({ kind: "path" as const, value, prefix: absolutePathDraft(commandDraft)?.token ?? "" })),
  ].slice(0, 6), [commandDraft, commandSuggestions, pathSuggestions]);
  const completionItemsRef = useRef<Completion[]>([]);
  completionItemsRef.current = completionItems;

  async function reconnectExitedSession() {
    if (onReconnect === undefined || manualReconnectBusy) return;
    setManualReconnectBusy(true);
    setProblem("");
    try {
      if (await onReconnect()) {
        control.current.now();
        return;
      }
      setProblem(t("terminal.manualReconnectFailed"));
    } finally {
      setManualReconnectBusy(false);
    }
  }

  useEffect(() => {
    const parsed = absolutePathDraft(commandDraft);
    if (session.alias === undefined || parsed === null) {
      setPathSuggestions([]);
      return;
    }
    let active = true;
    const timer = window.setTimeout(() => {
      void sftpApi.list(session.alias ?? "", parsed.parent).then(({ entries }) => {
        if (!active) return;
        const base = parsed.parent === "/" ? "/" : `${parsed.parent}/`;
        setPathSuggestions(entries
          .filter((entry) => entry.name.startsWith(parsed.basename))
          .slice(0, 5)
          .map((entry) => `${base}${entry.name}${entry.type === "directory" ? "/" : ""}`));
      }).catch(() => { if (active) setPathSuggestions([]); });
    }, 180);
    return () => { active = false; window.clearTimeout(timer); };
  }, [commandDraft, session.alias]);

  useEffect(() => {
    const openSearch = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.key.toLocaleLowerCase() !== "f") return;
      if (host.current === null || !host.current.contains(document.activeElement)) return;
      event.preventDefault();
      setSearchOpen(true);
    };
    window.addEventListener("keydown", openSearch);
    return () => window.removeEventListener("keydown", openSearch);
  }, []);

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const coarse = prefersNativeSelection((query) => window.matchMedia(query));

    const view = new Terminal({
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.state !== "exited",
      fontFamily: fontStack(font ?? ""),
      fontSize: fontSize ?? (window.matchMedia("(max-width: 767px)").matches ? 15 : 13),
      theme: terminalTheme(container, hasBackground),
      scrollback: 5000,
    });
    const fit = new FitAddon();
    view.loadAddon(fit);
    view.open(container);
    terminal.current = view;

    let stream: TerminalStream | null = null;
    let live = true;
    let stopped = false;
    let attempts = 0;
    let linkedAt = 0;
    let timer: ReturnType<typeof setInterval> | undefined;
    const decoder = new TextDecoder();

    let sent = "";
    const syncSize = () => {
      const size = `${view.cols}x${view.rows}`;
      if (stream === null || view.cols === 0 || view.rows === 0 || size === sent) return;
      sent = size;
      stream.resize(view.cols, view.rows);
    };

    const measure = () => {
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
      if (selectionHeldIn(container)) return;
      try {
        fit.fit();
      } catch {
        return;
      }
      syncSize();
    };
    measure();
    refit.current = measure;

    const scroll = newTouchScroll(view, () => cellHeight(view, container));
    const single = (event: TouchEvent): Touch | null =>
      event.touches.length === 1 ? (event.touches[0] ?? null) : null;
    const touchStart = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null && !selectionHeldIn(container)) scroll.start(finger.clientY);
    };
    const touchMove = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null && !selectionHeldIn(container)) scroll.move(finger.clientY);
    };
    container.addEventListener("touchstart", touchStart, { passive: true });
    container.addEventListener("touchmove", touchMove, { passive: true });


    const releaseImeKeys = coarse
      ? attachImeKeys({ container, textarea: view.textarea ?? container })
      : () => {};

    const detachOverlay = coarse ? attachSelectionOverlay(container, view) : () => {};

    const rememberInput = (data: string) => {
      const next = updateCommandDraft(commandDraftRef.current, data);
      commandDraftRef.current = next.draft;
      setCommandDraft(next.draft);
      if (next.completed.length > 0) setCommandHistory((current) => [...current, ...next.completed].slice(-200));
    };
    const typed = (data: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = applyModifiers(data, ctrl, alt);
      stream?.send(encoded);
      rememberInput(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    send.current = (label: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = encodeKey(label, ctrl, alt);
      stream?.send(encoded);
      rememberInput(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    acceptCompletion.current = (completion) => {
      if (completion.prefix === "" || !completion.value.startsWith(completion.prefix)) return;
      const suffix = completion.value.slice(completion.prefix.length);
      stream?.send(suffix);
      rememberInput(suffix);
      view.focus();
    };
    view.attachCustomKeyEventHandler((event) => {
      if (event.type !== "keydown" || event.key !== "Tab") return true;
      const suggestion = completionItemsRef.current[0];
      if (suggestion === undefined) return true;
      event.preventDefault();
      acceptCompletion.current(suggestion);
      return false;
    });
    view.onData(typed);

    let searchIndex = -1;
    searchStep.current = (direction) => {
      const lines: string[] = [];
      const buffer = view.buffer.active;
      for (let row = 0; row < buffer.length; row += 1) lines.push(buffer.getLine(row)?.translateToString(true) ?? "");
      const matches = findBufferMatches(lines, searchQueryRef.current);
      if (matches.length === 0) {
        searchIndex = -1;
        view.clearSelection();
        setSearchResult({ index: -1, total: 0 });
        return;
      }
      searchIndex = (searchIndex + direction + matches.length) % matches.length;
      const match = matches[searchIndex];
      if (match === undefined) return;
      view.select(match.column, match.row, match.length);
      view.scrollToLine(match.row);
      setSearchResult({ index: searchIndex, total: matches.length });
    };

    const attach = () => {
      clearInterval(timer);
      stopped = false;
      attempts += 1;
      setLink({ phase: "connecting", attempt: attempts });
      void api
        .terminalStreamTicket(session.id)
        .then((issued) => {
          if (!live || stopped) return;
          sent = "";
          linkedAt = Date.now();
          stream = openStream(issued.streamTicket, {
            onOutput: (chunk) => view.write(decoder.decode(chunk, { stream: true })),
            onExit: () => {
              view.options.cursorBlink = false;
              stopped = true;
              setLink({ phase: "live" });
              onExit?.();
            },
            onClose: () => {
              stream = null;
              retry();
            },
          });
          setLink({ phase: "live" });
          if (session.state !== "exited") view.focus();
          syncSize();
        })
        .catch((error: unknown) => {
          if (!live || stopped) return;
          if (failureCode(error) === "terminal_session_not_found") {
            setLink({ phase: "stopped", gone: true });
            return;
          }
          retry();
        });
    };

    const retry = () => {
      if (!live || stopped) return;
      if (linkedAt !== 0 && Date.now() - linkedAt > settled) attempts = 1;
      let left = backoff[Math.min(attempts - 1, backoff.length - 1)] ?? 1;
      const next = attempts + 1;
      setLink({ phase: "waiting", attempt: next, seconds: left });
      timer = setInterval(() => {
        left -= 1;
        if (left > 0) {
          setLink({ phase: "waiting", attempt: next, seconds: left });
          return;
        }
        attach();
      }, 1000);
    };

    control.current = {
      now: attach,
      stop: () => {
        stopped = true;
        clearInterval(timer);
        setLink({ phase: "stopped", gone: false });
      },
    };
    attach();

    const detachClipboard = attachTerminalClipboard({
      container,
      terminal: view,
      clipboard,
      coarsePointer: () => coarse,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
    });

    const observer = new ResizeObserver(measure);
    observer.observe(container);

    return () => {
      live = false;
      clearInterval(timer);
      observer.disconnect();
      container.removeEventListener("touchstart", touchStart);
      container.removeEventListener("touchmove", touchMove);
      releaseImeKeys();
      detachOverlay();
      detachClipboard();
      stream?.close();
      view.dispose();
      terminal.current = null;
      acceptCompletion.current = () => {};
      searchStep.current = () => {};
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id, api]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = fontStack(font ?? "");
    refit.current?.();
  }, [font]);

  return (
    <section aria-label={t("terminal.screenLabel", { title: session.title })} className="relative flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
        <span
          aria-hidden="true"
          className={`size-2 shrink-0 rounded-full ${
            session.state === "exited"
              ? "bg-ink-faint"
              : session.state === "reconnecting" || session.state === "connecting" || link.phase !== "live"
                ? "bg-notice-ink"
                : "bg-live"
          }`}
        />
        <p className="min-w-0 truncate font-mono text-xs font-semibold text-ink">{session.title}</p>
        {session.alias === undefined || session.alias === session.title ? null : (
          <span className="truncate rounded-md bg-surface px-2 py-0.5 font-mono text-[11px] text-ink-muted">
            {session.alias}
          </span>
        )}
        <button type="button" className="ml-auto rounded border border-control-line px-2 py-0.5 text-xs text-ink-muted hover:bg-select-fill" onClick={() => setSearchOpen((current) => !current)}>{t("terminal.search")}</button>
      </div>
      {searchOpen ? <div className="flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-1.5"><input autoFocus aria-label={t("terminal.searchInput")} value={searchQuery} onChange={(event) => { setSearchQuery(event.target.value); setSearchResult({ index: -1, total: 0 }); }} onKeyDown={(event) => { if (event.key === "Escape") setSearchOpen(false); else if (event.key === "Enter") searchStep.current(event.shiftKey ? -1 : 1); }} className="min-w-0 flex-1 rounded border border-control-line bg-control px-2 py-1 text-xs" placeholder={t("terminal.searchPlaceholder")} /><span className="w-16 text-center text-xs text-ink-muted">{searchResult.total === 0 ? t("terminal.searchNoResults") : `${searchResult.index + 1}/${searchResult.total}`}</span><button type="button" aria-label={t("terminal.searchPrevious")} className="rounded border border-control-line px-2 text-sm" onClick={() => searchStep.current(-1)}>↑</button><button type="button" aria-label={t("terminal.searchNext")} className="rounded border border-control-line px-2 text-sm" onClick={() => searchStep.current(1)}>↓</button><button type="button" aria-label={t("terminal.searchClose")} className="rounded px-2 text-sm" onClick={() => setSearchOpen(false)}>×</button></div> : null}
      {problem === "" ? null : (
        <p role="status" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {problem}
        </p>
      )}

      {session.state !== "reconnecting" ? null : (
        <p role="status" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {t("terminal.reconnectingAttempt", {
            attempt: String(session.reconnect?.attempt ?? 1),
            limit: String(session.reconnect?.limit ?? 1),
          })}
        </p>
      )}

      {session.problem === "" ? null : (
        <p role="alert" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {t(terminalProblemKey(session.problem))}
        </p>
      )}

      {link.phase === "live" ? null : (
        <div
          role="status"
          className="flex shrink-0 items-center gap-2 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink"
        >
          <p className="min-w-0 grow">
            {link.phase === "connecting"
              ? link.attempt === 1
                ? t("terminal.linkConnecting")
                : t("terminal.linkRetrying", { attempt: String(link.attempt) })
              : link.phase === "waiting"
                ? t("terminal.linkWaiting", { seconds: String(link.seconds), attempt: String(link.attempt) })
                : link.gone
                  ? t("terminal.linkGone")
                  : t("terminal.linkStopped")}
          </p>
          {link.phase === "stopped" && link.gone ? null : (
            <button
              type="button"
              disabled={link.phase === "connecting"}
              onClick={() => control.current.now()}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink disabled:opacity-50"
            >
              {t("terminal.linkNow")}
            </button>
          )}
          {link.phase === "stopped" ? null : (
            <button
              type="button"
              onClick={() => control.current.stop()}
              className="shrink-0 rounded border border-notice-line px-2 py-0.5 text-notice-ink"
            >
              {t("terminal.linkStop")}
            </button>
          )}
        </div>
      )}
      {session.exited === undefined ? null : (
        <div role="status" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-card px-3 py-1.5 text-xs text-ink-muted">
          <p className="min-w-0 grow">
            {session.exited.signal === ""
              ? t("terminal.exitedWithCode", { code: String(session.exited.code) })
              : t("terminal.exitedWithSignal", { signal: session.exited.signal })}
          </p>
          {session.kind !== "ssh" || session.alias === undefined || onReconnect === undefined ? null : (
            <button
              type="button"
              disabled={manualReconnectBusy}
              onClick={() => void reconnectExitedSession()}
              className="min-h-8 shrink-0 rounded border border-control-line bg-control px-3 py-1 font-medium text-ink hover:bg-select-fill disabled:opacity-50"
            >
              {t(manualReconnectBusy ? "terminal.manualReconnecting" : "terminal.manualReconnect")}
            </button>
          )}
        </div>
      )}


      <div
        ref={host}
        {...(palette === undefined || palette === "" ? {} : { "data-term-palette": palette })}
        {...(font === undefined || font === "" ? {} : { "data-term-font": font })}
        {...(hasBackground ? { "data-term-background": background ?? "" } : {})}
        style={
          hasBackground
            ? {
                "--ui-term-image": `url("${backgroundURL}")`,
                "--ui-term-tint": String(tint ?? defaultTint),
              } as CSSProperties
            : undefined
        }
        className="relative min-h-0 flex-1 bg-term-bg p-2"
      />
      {completionItems.length === 0 ? null : <div role="listbox" aria-label={t("terminal.completions")} className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar px-2 py-1">{completionItems.map((item) => <button key={`${item.kind}:${item.value}`} type="button" role="option" aria-selected="false" title={item.value} className="max-w-64 truncate rounded bg-select-fill px-2 py-1 font-mono text-xs text-ink hover:bg-accent/20" onClick={() => acceptCompletion.current(item)}><span className="mr-1 text-ink-muted">{item.kind === "command" ? t("terminal.commandSuggestion") : t("terminal.pathSuggestion")}</span>{item.value}</button>)}</div>}
      <KeyBar
        modifiers={modifiers}
        onToggle={(name) => setModifiers((current) => ({ ...current, [name]: !current[name] }))}
        onKey={(label) => send.current(label)}
      />
    </section>
  );
}

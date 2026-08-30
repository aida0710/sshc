import { useEffect, useRef, useState, type CSSProperties } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { connectionProgressText } from "./progress";
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
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";
import { validSearchPattern, type TerminalSearchSettings } from "./search";
import { terminalProblemKey } from "./sessions";
import { agentName, agentStatusLabel, terminalDisplayTitle, terminalSubtitle } from "./agentPresentation";
import { recentBufferText } from "./buffer";
import { attachOsc52Clipboard } from "./osc52";
import { attachKittyKeyboardProtocol, encodeIntlYen } from "./kittyKeyboard";
import { findTerminalLinks, modifierOpensLink, osc8Link } from "./links";
import { openTerminalURL, TerminalLinkPopover, type RemotePathAction, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { TerminalQuickCommands } from "./TerminalQuickCommands";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";
import { TerminalPortForwards } from "./TerminalPortForwards";
import { attachWebglRenderer } from "./webgl";
import { Icon } from "../ui/icons";

type TerminalViewProps = {
  session: TerminalSession;
  api?: Pick<IntegrationsApi, "terminalStreamTicket">;
  onExit?: () => void;
  onReconnect?: () => Promise<boolean>;
  onResumeAgent?: (placement: "same-pane" | "new-pane") => Promise<boolean>;
  copyOnSelect?: boolean;
  fontSize?: number;
  rightClickPaste?: boolean;
  palette?: string;
  background?: string;
  tint?: number;
  font?: string;
  onOpenRemotePath?: (alias: string, path: string, action: RemotePathAction) => void;
  osc52Enabled?: boolean;
  scrollbackLines?: number;
  onOsc52Change?: (enabled: boolean) => void | Promise<void>;
  onForwardsChanged?: () => void | Promise<void>;
  jisYenBackslash?: boolean;
};

type Link =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  | { phase: "stopped"; gone: boolean };

const backoff = [1, 2, 4, 8, 15];

const settled = 10_000;

export function TerminalView({
  session,
  api = integrationsApi,
  onExit,
  onReconnect,
  onResumeAgent,
  copyOnSelect = true,
  fontSize,
  rightClickPaste = true,
  palette,
  font,
  background,
  tint,
  onOpenRemotePath,
  osc52Enabled: initialOsc52Enabled = false,
  scrollbackLines = 5000,
  onOsc52Change,
  onForwardsChanged,
  jisYenBackslash = false,
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
  const [searchCaseSensitive, setSearchCaseSensitive] = useState(false);
  const [searchRegex, setSearchRegex] = useState(false);
  const [searchInvalid, setSearchInvalid] = useState(false);
  const [searchResult, setSearchResult] = useState({ index: -1, total: 0 });
  const searchQueryRef = useRef("");
  searchQueryRef.current = searchQuery;
  const searchSettingsRef = useRef<TerminalSearchSettings>({ caseSensitive: false, regex: false });
  searchSettingsRef.current = { caseSensitive: searchCaseSensitive, regex: searchRegex };
  const searchStep = useRef<(direction: 1 | -1) => void>(() => {});
  const searchRefresh = useRef<() => void>(() => {});
  const searchClear = useRef<() => void>(() => {});
  const copyContext = useRef<() => void>(() => {});
  const [osc52Enabled, setOsc52Enabled] = useState(initialOsc52Enabled);
  const osc52EnabledRef = useRef(initialOsc52Enabled);
  osc52EnabledRef.current = osc52Enabled;
  const intlYenRef = useRef(jisYenBackslash);
  intlYenRef.current = jisYenBackslash;
  const [terminalNotice, setTerminalNotice] = useState("");

  const [quickCommandsOpen, setQuickCommandsOpen] = useState(false);
  const [quickCommandSelection, setQuickCommandSelection] = useState("");
  const [overflowOpen, setOverflowOpen] = useState(false);
  const [portForwardsOpen, setPortForwardsOpen] = useState(false);
  const [linkSelection, setLinkSelection] = useState<TerminalLinkSelection | null>(null);
  const control = useRef<{ now: () => void; stop: () => void }>({ now: () => {}, stop: () => {} });

  const [modifiers, setModifiers] = useState<Modifiers>({ ctrl: false, alt: false });
  const armed = useRef<Modifiers>(modifiers);
  armed.current = modifiers;
  const send = useRef<(label: string) => void>(() => {});
  const sendInput = useRef<(text: string) => void>(() => {});

  useEffect(() => {
    setOsc52Enabled(initialOsc52Enabled);
  }, [initialOsc52Enabled, session.id]);

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

  async function resumeAgent(placement: "same-pane" | "new-pane") {
    if (onResumeAgent === undefined || manualReconnectBusy) return;
    setManualReconnectBusy(true);
    setProblem("");
    try {
      if (await onResumeAgent(placement)) {
        if (placement === "same-pane") control.current.now();
        return;
      }
      setProblem(t("terminal.agentResumeFailed"));
    } finally {
      setManualReconnectBusy(false);
    }
  }

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
    if (searchOpen) searchRefresh.current();
    else searchClear.current();
  }, [searchOpen, searchQuery, searchCaseSensitive, searchRegex]);

  useEffect(() => {
    if (terminalNotice === "") return;
    const timer = window.setTimeout(() => setTerminalNotice(""), 2500);
    return () => window.clearTimeout(timer);
  }, [terminalNotice]);

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const coarse = prefersNativeSelection((query) => window.matchMedia(query));

    let view: Terminal;
    view = new Terminal({
      allowProposedApi: true,
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.state !== "exited",
      fontFamily: fontStack(font ?? ""),
      fontSize: fontSize ?? (window.matchMedia("(max-width: 767px)").matches ? 15 : 13),
      theme: terminalTheme(container, hasBackground),
      scrollback: scrollbackLines,
      linkHandler: {
        activate: (event, target, range) => {
          const line = view.buffer.active.getLine(range.start.y - 1)?.translateToString(true) ?? "";
          const visible = line.slice(range.start.x - 1, range.end.x);
          const link = osc8Link(target, visible, range.start.x - 1, range.end.x);
          if (link === null) return;
          if (modifierOpensLink(event)) {
            openTerminalURL(link.target);
            return;
          }
          setLinkSelection({ link, x: event.clientX + 8, y: event.clientY + 8 });
        },
      },
    });
    const fit = new FitAddon();
    const search = new SearchAddon({ highlightLimit: 1000 });
    view.loadAddon(fit);
    view.loadAddon(search);
    view.open(container);
    let webgl: { dispose(): void } | null = null;
    let terminalDisposed = false;
    void attachWebglRenderer(view).then((attached) => {
      if (terminalDisposed) attached?.dispose();
      else webgl = attached;
    });
    terminal.current = view;

    const searchResultSubscription = search.onDidChangeResults((result) => {
      setSearchResult({ index: result.resultIndex, total: result.resultCount });
    });

    const searchOptions = (incremental: boolean) => {
      const style = getComputedStyle(container);
      const match = style.getPropertyValue("--ui-term-yellow").trim();
      const active = style.getPropertyValue("--ui-term-bright-yellow").trim();
      return {
        ...searchSettingsRef.current,
        incremental,
        decorations: {
          matchBackground: match,
          matchBorder: active,
          matchOverviewRuler: active,
          activeMatchBackground: active,
          activeMatchBorder: match,
          activeMatchColorOverviewRuler: match,
        },
      };
    };
    const runSearch = (direction: 1 | -1, incremental: boolean) => {
      const query = searchQueryRef.current;
      if (!validSearchPattern(query, searchSettingsRef.current)) {
        search.clearDecorations();
        view.clearSelection();
        setSearchInvalid(true);
        setSearchResult({ index: -1, total: 0 });
        return;
      }
      setSearchInvalid(false);
      if (query === "") {
        search.clearDecorations();
        view.clearSelection();
        setSearchResult({ index: -1, total: 0 });
        return;
      }
      if (direction === 1) search.findNext(query, searchOptions(incremental));
      else search.findPrevious(query, searchOptions(false));
    };
    searchStep.current = (direction) => runSearch(direction, false);
    searchRefresh.current = () => runSearch(1, true);
    searchClear.current = () => {
      search.clearDecorations();
      view.clearSelection();
      setSearchResult({ index: -1, total: 0 });
      setSearchInvalid(false);
    };
    copyContext.current = () => {
      const text = recentBufferText(view.buffer.active);
      if (text === "") {
        setTerminalNotice(t("terminal.copyContextEmpty"));
        return;
      }
      void clipboard.writeText(text)
        .then(() => setTerminalNotice(t("terminal.copyContextDone")))
        .catch(() => setProblem(t("terminal.clipboardRefused")));
    };

    const kittyKeyboard = attachKittyKeyboardProtocol(view.parser);
    const detachOsc52 = attachOsc52Clipboard({
      parser: view.parser,
      enabled: () => osc52EnabledRef.current,
      writeText: (text) => clipboard.writeText(text),
      copied: () => setTerminalNotice(t("terminal.osc52Copied")),
      refused: () => setProblem(t("terminal.clipboardRefused")),
    });
    const terminalLinks = view.registerLinkProvider({
      provideLinks: (bufferLineNumber, callback) => {
        const line = view.buffer.active.getLine(bufferLineNumber - 1)?.translateToString(true) ?? "";
        const matches = findTerminalLinks(line, session.kind === "ssh");
        callback(matches.length === 0 ? undefined : matches.map((match) => ({
          text: match.text,
          range: {
            start: { x: match.start + 1, y: bufferLineNumber },
            end: { x: match.end, y: bufferLineNumber },
          },
          activate: (event: MouseEvent) => {
            if (match.kind === "url" && modifierOpensLink(event)) {
              openTerminalURL(match.target);
              return;
            }
            setLinkSelection({
              link: match,
              x: event.clientX + 8,
              y: event.clientY + 8,
            });
          },
        })));
      },
    });

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

    const fitAndSync = () => {
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
      try {
        fit.fit();
      } catch {
        return;
      }
      syncSize();
      view.refresh(0, Math.max(0, view.rows - 1));
    };
    const measure = () => {
      if (selectionHeldIn(container)) return;
      fitAndSync();
    };
    measure();
    refit.current = fitAndSync;

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

    const typed = (data: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = applyModifiers(data, ctrl, alt);
      sendInput.current(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    sendInput.current = (text) => stream?.send(text);
    send.current = (label: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = encodeKey(label, ctrl, alt);
      stream?.send(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    view.onData(typed);

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
      paste: (text) => stream?.send(prepareTerminalPaste(text, view.modes.bracketedPasteMode)),
      clipboard,
      coarsePointer: () => coarse,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
      enhancedKey: (event) => encodeIntlYen(event, intlYenRef.current) ?? kittyKeyboard.encode(event),
      sendEnhancedKey: (sequence) => stream?.send(sequence),
    });

    const observer = new ResizeObserver(measure);
    observer.observe(container);

    return () => {
      live = false;
      terminalDisposed = true;
      clearInterval(timer);
      observer.disconnect();
      container.removeEventListener("touchstart", touchStart);
      container.removeEventListener("touchmove", touchMove);
      releaseImeKeys();
      detachOverlay();
      detachClipboard();
      detachOsc52();
      kittyKeyboard.dispose();
      terminalLinks.dispose();
      webgl?.dispose();
      searchResultSubscription.dispose();
      stream?.close();
      view.dispose();
      terminal.current = null;
      sendInput.current = () => {};
      searchStep.current = () => {};
      searchRefresh.current = () => {};
      searchClear.current = () => {};
      copyContext.current = () => {};
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

  const connectionStatus = session.state === "connecting" || session.state === "reconnecting"
    ? connectionProgressText(t, session)
    : session.state === "connected"
      ? t("terminal.connected")
      : t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
  const displayTitle = terminalDisplayTitle(session);
  const subtitle = terminalSubtitle(session);
  const agentStatus = agentStatusLabel(t, session);
  const remoteAlias = session.kind === "ssh" ? session.alias : undefined;

  return (
    <section aria-label={t("terminal.screenLabel", { title: displayTitle })} className="relative flex min-h-0 flex-1 flex-col">
      <div className="relative flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
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
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <p className="min-w-0 flex-1 truncate text-xs font-semibold text-ink">{displayTitle}</p>
            {agentStatus === "" ? null : (
              <span role="status" className="shrink-0 truncate text-[11px] font-medium text-ink-muted">{agentStatus}</span>
            )}
          </div>
          <div className="flex min-w-0 items-center gap-2 text-[11px] text-ink-muted">
            <span className="min-w-0 truncate font-mono">{subtitle}</span>
            {session.state === "connected" && agentStatus !== "" ? null : (
              <span role="status" className="shrink-0">{connectionStatus}</span>
            )}
          </div>
        </div>
        <button
          type="button"
          aria-label={t("terminal.search")}
          title={t("terminal.search")}
          className="rounded border border-control-line px-2 py-1 text-xs text-ink-muted hover:bg-select-fill"
          onClick={() => setSearchOpen((current) => !current)}
        >
          <Icon name="search" className="size-3.5" />
        </button>
        <button
          type="button"
          aria-label={t("terminal.moreActions")}
          aria-expanded={overflowOpen}
          className="rounded border border-control-line px-2 py-1 text-ink-muted hover:bg-select-fill"
          onClick={() => setOverflowOpen((current) => !current)}
        >
          <Icon name="moreHorizontal" className="size-3.5" />
        </button>
        {overflowOpen ? (
          <TerminalOverflowMenu
            osc52Enabled={osc52Enabled}
            onQuickCommands={() => {
              setQuickCommandSelection(terminal.current?.getSelection() ?? "");
              setQuickCommandsOpen(true);
            }}
            onPortForwarding={session.kind === "ssh" ? () => setPortForwardsOpen(true) : undefined}
            onCopyContext={() => copyContext.current()}
            onToggleOsc52={async () => {
              const next = !osc52Enabled;
              try {
                await onOsc52Change?.(next);
                setOsc52Enabled(next);
                setTerminalNotice(t(next ? "terminal.osc52Enabled" : "terminal.osc52Disabled"));
              } catch {
                setTerminalNotice(t("terminal.settingsSaveFailed"));
              }
            }}
            onClose={() => setOverflowOpen(false)}
          />
        ) : null}
      </div>
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

      {session.agent?.resumable !== true || session.agent.state !== "unknown" || onResumeAgent === undefined ? null : (
        <div role="status" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          <p className="min-w-0 grow">{t("terminal.agentResumeAvailable", { agent: agentName(session.agent.kind) })}</p>
          <button
            type="button"
            disabled={manualReconnectBusy}
            onClick={() => void resumeAgent("same-pane")}
            className="min-h-8 rounded border border-control-line bg-control px-3 py-1 font-medium text-ink hover:bg-select-fill disabled:opacity-50"
          >
            {t("terminal.agentResumeSamePane")}
          </button>
          <button
            type="button"
            disabled={manualReconnectBusy}
            onClick={() => void resumeAgent("new-pane")}
            className="min-h-8 rounded border border-control-line bg-control px-3 py-1 font-medium text-ink hover:bg-select-fill disabled:opacity-50"
          >
            {t("terminal.agentResumeNewPane")}
          </button>
        </div>
      )}
      <div className="relative min-h-0 flex-1 overflow-hidden bg-term-bg">
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
          className="absolute inset-0 bg-term-bg p-2"
        />
        {searchOpen ? (
          <div className="absolute inset-x-2 top-2 z-20 flex items-center gap-1.5 rounded-lg border border-line bg-toolbar/95 p-1.5 shadow-lg backdrop-blur sm:left-auto sm:w-[34rem]">
            <input
              autoFocus
              aria-label={t("terminal.searchInput")}
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape") setSearchOpen(false);
                else if (event.key === "Enter") searchStep.current(event.shiftKey ? -1 : 1);
              }}
              className="min-w-0 flex-1 rounded border border-control-line bg-control px-2 py-1 text-xs"
              placeholder={t("terminal.searchPlaceholder")}
            />
            <button
              type="button"
              aria-label={t("terminal.searchCaseSensitive")}
              aria-pressed={searchCaseSensitive}
              className={`rounded border px-1.5 py-1 text-xs ${searchCaseSensitive ? "border-accent bg-accent/10 text-accent" : "border-control-line text-ink-muted"}`}
              onClick={() => setSearchCaseSensitive((current) => !current)}
            >
              Aa
            </button>
            <button
              type="button"
              aria-label={t("terminal.searchRegex")}
              aria-pressed={searchRegex}
              className={`rounded border px-1.5 py-1 font-mono text-xs ${searchRegex ? "border-accent bg-accent/10 text-accent" : "border-control-line text-ink-muted"}`}
              onClick={() => setSearchRegex((current) => !current)}
            >
              .*
            </button>
            <span role="status" className={`w-14 text-center text-[11px] ${searchInvalid ? "text-danger" : "text-ink-muted"}`}>
              {searchInvalid
                ? t("terminal.searchInvalidRegex")
                : searchResult.total === 0
                  ? t("terminal.searchNoResults")
                  : `${searchResult.index + 1}/${searchResult.total}`}
            </span>
            <button type="button" aria-label={t("terminal.searchPrevious")} className="rounded border border-control-line px-2 py-0.5 text-sm" onClick={() => searchStep.current(-1)}>↑</button>
            <button type="button" aria-label={t("terminal.searchNext")} className="rounded border border-control-line px-2 py-0.5 text-sm" onClick={() => searchStep.current(1)}>↓</button>
            <button type="button" aria-label={t("terminal.searchClose")} className="rounded px-2 py-0.5 text-sm" onClick={() => setSearchOpen(false)}>×</button>
          </div>
        ) : null}
        {terminalNotice === "" ? null : (
          <p role="status" className="absolute bottom-3 right-3 z-20 max-w-[min(24rem,calc(100%-1.5rem))] rounded border border-line bg-toolbar/95 px-3 py-2 text-xs text-ink shadow-lg">
            {terminalNotice}
          </p>
        )}
      </div>
      {quickCommandsOpen ? (
        <TerminalQuickCommands
          session={session}
          initialCommand={quickCommandSelection}
          onClose={() => setQuickCommandsOpen(false)}
        />
      ) : null}
      {portForwardsOpen ? (
        <TerminalPortForwards
          session={session}
          {...(onForwardsChanged === undefined ? {} : { onChanged: onForwardsChanged })}
          onClose={() => setPortForwardsOpen(false)}
        />
      ) : null}
      {linkSelection === null ? null : (
        <TerminalLinkPopover
          selection={linkSelection}
          onClose={() => setLinkSelection(null)}
          {...(remoteAlias !== undefined && onOpenRemotePath !== undefined
            ? { onRemotePath: (path: string, action: RemotePathAction) => onOpenRemotePath(remoteAlias, path, action) }
            : {})}
        />
      )}
      <KeyBar
        modifiers={modifiers}
        onToggle={(name) => setModifiers((current) => ({ ...current, [name]: !current[name] }))}
        onKey={(label) => send.current(label)}
      />
    </section>
  );
}

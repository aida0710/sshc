import { useEffect, useRef, useState, type CSSProperties } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { failureCode } from "../api/client";
import { terminalSessionsApi, type TerminalSession, type TerminalSessionsApi } from "../api/terminalSessions";
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
import { cellHeight, observeTerminalSize, syncTerminalInputPosition } from "./metrics";
import { attachTouchScroll } from "./touchScroll";
import { KeyBar, applyModifiers, encodeKey, type Modifiers } from "./KeyBar";
import { openStream, type TerminalStream } from "./stream";
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";
import { recentBufferText } from "./buffer";
import { attachOsc52Clipboard } from "./osc52";
import { attachKittyKeyboardProtocol, encodeIntlYen } from "./kittyKeyboard";
import { modifierOpensLink, osc8Link } from "./links";
import { attachLinkProvider } from "./linkProvider";
import { openTerminalURL, TerminalLinkPopover, type RemotePathAction, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { TerminalQuickCommands } from "./TerminalQuickCommands";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";
import { TerminalPortForwards } from "./TerminalPortForwards";
import { attachWebglRenderer } from "./webgl";
import { attachOSC7Directory } from "./osc7";
import { attachCommandMarkers } from "./commandMarkers";
import { showBrowserNotification } from "./terminalNotifications";
import { applyTerminalRuntimeOptions } from "./runtimeOptions";
import { Icon } from "../ui/icons";
import { useTerminalSearch } from "./useTerminalSearch";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { TerminalStatusBanners } from "./TerminalStatusBanners";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import type { StreamLink } from "./streamLink";
import { inspectTerminalPaste } from "./pasteGuard";
import { TerminalPasteDialog } from "./TerminalPasteDialog";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { cursorAnimationEnabled, reducedMotionQuery } from "../ui/reducedMotion";

type TerminalViewProps = {
  session: TerminalSession;
  searchShortcutActive?: boolean;
  api?: Pick<TerminalSessionsApi, "terminalStreamTicket">;
  onExit?: () => void;
  onReconnect?: () => Promise<boolean>;
  onStopReconnect?: () => Promise<boolean>;
  copyOnSelect?: boolean;
  fontSize?: number;
  rightClickPaste?: boolean;
  webgl?: boolean;
  palette?: string;
  background?: string;
  tint?: number;
  font?: string;
  onOpenRemotePath?: (alias: string, path: string, action: RemotePathAction) => void;
  osc52Enabled?: boolean;
  // vpnProfile は、この接続が通るVPN経路の名前である。空なら通らない。
  vpnProfile?: string;
  scrollbackLines?: number;
  onOsc52Change?: (enabled: boolean) => void | Promise<void>;
  onForwardsChanged?: () => void | Promise<void>;
  jisYenBackslash?: boolean;
};

const backoff = [1, 2, 4, 8, 15];

const settled = 10_000;

export function TerminalView({
  session,
  searchShortcutActive,
  api = terminalSessionsApi,
  onExit,
  onReconnect,
  onStopReconnect,
  copyOnSelect = true,
  fontSize,
  rightClickPaste = true,
  webgl: webglEnabled = true,
  palette,
  font,
  background,
  tint,
  onOpenRemotePath,
  osc52Enabled: initialOsc52Enabled = false,
  vpnProfile = "",
  scrollbackLines = 5000,
  onOsc52Change,
  onForwardsChanged,
  jisYenBackslash = false,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const mobile = useMediaQuery(mobileViewportQuery);
  const reducedMotionRef = useRef(reducedMotion);
  reducedMotionRef.current = reducedMotion;
  const host = useRef<HTMLDivElement>(null);
  const region = useRef<HTMLElement>(null);
  const backgroundURL = useBackgroundImage(background ?? "");
  const backgroundConfigured = (background ?? "") !== "";
  const hasBackground = backgroundURL !== "";
  const refit = useRef<(() => void) | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const clipboardSettings = useRef<TerminalClipboardSettings>({ copyOnSelect, rightClickPaste });
  clipboardSettings.current = { copyOnSelect, rightClickPaste };
  const [problem, setProblem] = useState("");
  const [link, setLink] = useState<StreamLink>({ phase: "connecting", attempt: 1 });
  const search = useTerminalSearch({ shortcutActive: searchShortcutActive, region });
  const copyContext = useRef<() => void>(() => {});
  const [osc52Enabled, setOsc52Enabled] = useState(initialOsc52Enabled);
  const osc52EnabledRef = useRef(initialOsc52Enabled);
  osc52EnabledRef.current = osc52Enabled;
  const intlYenRef = useRef(jisYenBackslash);
  intlYenRef.current = jisYenBackslash;
  const [terminalNotice, setTerminalNotice] = useState("");
  const [pendingPaste, setPendingPaste] = useState<{
    sessionID: string;
    raw: string;
  } | null>(null);
  const sendPaste = useRef<(text: string) => void>(() => {});
  const [currentDirectory, setCurrentDirectory] = useState("");

  const [quickCommandsOpen, setQuickCommandsOpen] = useState(false);
  const [quickCommandSelection, setQuickCommandSelection] = useState("");
  const [overflowOpen, setOverflowOpen] = useState(false);
  const overflowTrigger = useRef<HTMLButtonElement>(null);
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
    setCurrentDirectory("");
    setPendingPaste(null);
  }, [initialOsc52Enabled, session.id, session.state]);

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
      allowTransparency: backgroundConfigured,
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
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
    const searchAddon = new SearchAddon({ highlightLimit: 1000 });
    view.loadAddon(fit);
    view.loadAddon(searchAddon);
    view.open(container);
    let webgl: { dispose(): void } | null = null;
    let terminalDisposed = false;
    void attachWebglRenderer(view, {
      enabled: webglEnabled,
      backgroundImage: backgroundConfigured,
    }).then((attached) => {
      if (terminalDisposed) attached?.dispose();
      else webgl = attached;
    });
    terminal.current = view;

    const unbindSearch = search.bind(view, searchAddon, container);
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
    const osc7Directory = attachOSC7Directory(view.parser, setCurrentDirectory);
    const commandMarkers = attachCommandMarkers(view, ({ durationMilliseconds }) => {
      if (durationMilliseconds < 30_000 || !document.hidden) return;
      showBrowserNotification({
        title: "sshc",
        body: t("terminal.longCommandCompleted", {
          subject: terminalDisplayTitle(session),
          seconds: String(Math.round(durationMilliseconds / 1000)),
        }),
        tag: `sshc-command-${session.id}`,
      });
    });
    const terminalLinks = attachLinkProvider(view, {
      remote: session.kind === "ssh",
      open: openTerminalURL,
      select: (link, event) => setLinkSelection({ link, x: event.clientX + 8, y: event.clientY + 8 }),
    });

    let stream: TerminalStream | null = null;
    let live = true;
    let stopped = false;
    let attempts = 0;
    let linkedAt = 0;
    let timer: ReturnType<typeof setInterval> | undefined;
    let decoder = new TextDecoder();
    let cursor: number | undefined;

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
        if (coarse) syncTerminalInputPosition(view);
      } catch {
        return;
      }
      syncSize();
      view.refresh(0, Math.max(0, view.rows - 1));
    };
    // Keyboard/orientation changes must resize the PTY even while text is
    // selected; the selection overlay releases its handles when its shape changes.
    const measure = fitAndSync;
    measure();
    refit.current = fitAndSync;

    const detachTouchScroll = attachTouchScroll(container, view, () => cellHeight(view, container), {
      selectionHeld: () => selectionHeldIn(container),
      reducedMotion: () => reducedMotionRef.current,
    });

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
        .terminalStreamTicket(session.id, cursor)
        .then((issued) => {
          if (!live || stopped) return;
          sent = "";
          linkedAt = Date.now();
          stream = openStream(issued.streamTicket, {
            onReplay: (replay) => {
              cursor = replay.start;
              if (replay.truncated) {
                decoder = new TextDecoder();
                // The retained ring may begin in the middle of CSI or OSC.
                // CAN makes xterm abandon that parser state without clearing
                // the visible screen as a full terminal reset would.
                view.write("\u0018");
                setTerminalNotice(t("terminal.replayTruncated"));
              }
            },
            onOutput: (chunk) => {
              // A confirmation belongs to the exact terminal context the user
              // reviewed. Any output can be a new prompt or the reconnect
              // notice for a replacement shell, so require a fresh paste.
              setPendingPaste(null);
              view.write(decoder.decode(chunk, { stream: true }));
              cursor = (cursor ?? 0) + chunk.byteLength;
            },
            onExit: () => {
              view.options.cursorBlink = false;
              stopped = true;
              setPendingPaste(null);
              setLink({ phase: "live" });
              onExit?.();
            },
            onClose: () => {
              stream = null;
              setPendingPaste(null);
              retry();
            },
          });
          setLink({ phase: "live" });
          if (!coarse && session.state !== "exited" && !search.hasFocus()) {
            view.focus();
          }
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

    sendPaste.current = (text) => stream?.send(prepareTerminalPaste(text, view.modes.bracketedPasteMode));
    const detachClipboard = attachTerminalClipboard({
      container,
      terminal: view,
      paste: (text) => {
        const inspection = inspectTerminalPaste(text);
        if (inspection.requiresConfirmation) {
          setPendingPaste({ sessionID: session.id, raw: text });
          return;
        }
        sendPaste.current(text);
      },
      clipboard,
      coarsePointer: () => coarse,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
      enhancedKey: (event) => encodeIntlYen(event, intlYenRef.current) ?? kittyKeyboard.encode(event),
      sendEnhancedKey: (sequence) => stream?.send(sequence),
    });

    const stopObservingSize = observeTerminalSize(view, container, measure);

    return () => {
      live = false;
      terminalDisposed = true;
      clearInterval(timer);
      stopObservingSize();
      detachTouchScroll();
      releaseImeKeys();
      detachOverlay();
      detachClipboard();
      detachOsc52();
      osc7Directory.dispose();
      commandMarkers.dispose();
      kittyKeyboard.dispose();
      terminalLinks.dispose();
      webgl?.dispose();
      unbindSearch();
      stream?.close();
      view.dispose();
      terminal.current = null;
      sendInput.current = () => {};
      sendPaste.current = () => {};
      copyContext.current = () => {};
    };
    // The terminal is built once per session; settings, callbacks and
    // translations it reads are applied by the effects below without
    // recreating the addon stack and the WebSocket.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id, api, backgroundConfigured, webglEnabled]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = fontStack(font ?? "");
    refit.current?.();
  }, [font]);

  useEffect(() => {
    if (terminal.current === null) return;
    const changed = applyTerminalRuntimeOptions(terminal.current.options, {
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontSize: fontSize ?? (window.matchMedia("(max-width: 767px)").matches ? 15 : 13),
      scrollback: scrollbackLines,
    });
    if (changed.refit) refit.current?.();
  }, [fontSize, reducedMotion, scrollbackLines, session.state]);

  const connectionStatus = session.state === "connecting" || session.state === "reconnecting"
    ? connectionProgressText(t, session)
    : session.state === "connected"
      ? t("terminal.connected")
      : t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
  const displayTitle = terminalDisplayTitle(session);
  const subtitle = terminalSubtitle(session);
  const remoteAlias = session.kind === "ssh" ? session.alias : undefined;

  return (
    <section ref={region} aria-label={t("terminal.screenLabel", { title: displayTitle })} className="relative flex min-h-0 flex-1 flex-col">
      <div className={`relative flex shrink-0 items-center gap-2 border-b border-line bg-toolbar px-2 ${mobile ? "py-0" : "h-8"}`}>
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
        <div className="min-w-0 flex-1 md:flex md:items-center md:gap-2">
          <div className="flex min-w-0 items-center gap-2 md:shrink-0">
            <p className="min-w-0 flex-1 truncate text-xs font-semibold text-ink md:max-w-48 md:flex-none">{displayTitle}</p>
          </div>
          <div className="flex min-w-0 items-center gap-2 text-[11px] text-ink-muted md:flex-1">
            <span className="min-w-0 truncate font-mono">{subtitle}</span>
            <span role="status" className="shrink-0">{connectionStatus}</span>
            <VPNProfileChip name={vpnProfile} />
          </div>
        </div>
        <button
          type="button"
          aria-label={t("terminal.search")}
          title={t("terminal.search")}
          className={`flex shrink-0 items-center justify-center rounded border border-control-line text-xs text-ink-muted hover:bg-select-fill active:bg-select-fill focus:bg-select-fill focus:outline-none ${mobile ? "size-11" : "size-6"}`}
          onClick={search.toggle}
        >
          <Icon name="search" className="size-3.5" />
        </button>
        <button
          ref={overflowTrigger}
          type="button"
          aria-label={t("terminal.moreActions")}
          aria-expanded={overflowOpen}
          className={`flex shrink-0 items-center justify-center rounded border border-control-line text-ink-muted hover:bg-select-fill active:bg-select-fill focus:bg-select-fill focus:outline-none ${mobile ? "size-11" : "size-6"}`}
          onClick={() => setOverflowOpen((current) => !current)}
        >
          <Icon name="moreHorizontal" className="size-3.5" />
        </button>
        {overflowOpen ? (
          <TerminalOverflowMenu
            triggerRef={overflowTrigger}
            osc52Enabled={osc52Enabled}
            onQuickCommands={() => {
              setQuickCommandSelection(terminal.current?.getSelection() ?? "");
              setQuickCommandsOpen(true);
            }}
            onPortForwarding={session.kind === "ssh" ? () => setPortForwardsOpen(true) : undefined}
            onOpenRemoteDirectory={session.kind === "ssh" && session.alias !== undefined && currentDirectory !== "" && onOpenRemotePath !== undefined
              ? () => onOpenRemotePath(session.alias!, currentDirectory, "browse")
              : undefined}
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
      <TerminalStatusBanners
        session={session}
        problem={problem}
        link={link}
        onLinkNow={() => control.current.now()}
        onLinkStop={() => control.current.stop()}
        {...(onStopReconnect === undefined ? {} : { onStopReconnect })}
        {...(onReconnect === undefined ? {} : { onReconnect })}
      />

      <div className="relative min-h-0 flex-1 overflow-clip bg-term-bg">
        <div
          ref={host}
          data-terminal-host=""
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
          className="absolute inset-0 bg-term-bg"
        />
        {search.open ? <TerminalSearchBar search={search} mobile={mobile} /> : null}
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
          returnFocusRef={overflowTrigger}
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
      {pendingPaste === null || pendingPaste.sessionID !== session.id ? null : (
        <TerminalPasteDialog
          target={session.alias ?? session.title}
          text={pendingPaste.raw}
          onCancel={() => setPendingPaste(null)}
          onPaste={(raw) => {
            setPendingPaste(null);
            sendPaste.current(raw);
          }}
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

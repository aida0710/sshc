import { useEffect, useRef, useState, type CSSProperties } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
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
import { showBrowserNotification } from "../ui/browserNotifications";
import { applyTerminalRuntimeOptions } from "./runtimeOptions";
import { Icon } from "../ui/icons";
import { keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { useTerminalSearch } from "./useTerminalSearch";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { TerminalStatusBanners } from "./TerminalStatusBanners";
import { VPNProfileChip } from "../vpn/VPNProfileChip";
import { createStreamLink, type StreamLink, type StreamLinkController } from "./streamLink";
import { defaultBrowserScrollbackLines } from "./browserScrollback";
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

// Long enough to read a one-line notice such as "Copied" without it lingering
// over the output.
const terminalNoticeMs = 2500;

// A command shorter than this usually finishes while the user is still
// watching, so a desktop notification for it would only be noise.
const longCommandNotifyMs = 30_000;

// The link popover opens just below and to the right of the pointer so it does
// not cover the link that was clicked.
const linkPopoverOffsetPx = 8;

// When the settings leave the size unset, a phone (mobileViewportQuery, the
// same condition as its toolbar and KeyBar) gets a larger size so the text
// stays legible on its small screen.
const mobileTerminalFontSize = 15;
const desktopTerminalFontSize = 13;

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
  scrollbackLines = defaultBrowserScrollbackLines,
  onOsc52Change,
  onForwardsChanged,
  jisYenBackslash = false,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const mobile = useMediaQuery(mobileViewportQuery);
  const resolvedFontSize = fontSize ?? (mobile ? mobileTerminalFontSize : desktopTerminalFontSize);
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
    sessionId: string;
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
  const streamLinkController = useRef<StreamLinkController | null>(null);

  const [modifiers, setModifiers] = useState<Modifiers>({ ctrl: false, alt: false });
  const armed = useRef<Modifiers>(modifiers);
  armed.current = modifiers;
  const send = useRef<(name: string) => void>(() => {});
  const sendInput = useRef<(text: string) => void>(() => {});

  useEffect(() => {
    setOsc52Enabled(initialOsc52Enabled);
    setCurrentDirectory("");
    setPendingPaste(null);
  }, [initialOsc52Enabled, session.id, session.state]);

  useEffect(() => {
    if (terminalNotice === "") return;
    const timer = window.setTimeout(() => setTerminalNotice(""), terminalNoticeMs);
    return () => window.clearTimeout(timer);
  }, [terminalNotice]);

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const coarse = prefersNativeSelection((query) => window.matchMedia(query));
    const selectLinkAt = (link: TerminalLinkSelection["link"], event: MouseEvent) =>
      setLinkSelection({ link, x: event.clientX + linkPopoverOffsetPx, y: event.clientY + linkPopoverOffsetPx });

    let view: Terminal;
    view = new Terminal({
      allowProposedApi: true,
      allowTransparency: backgroundConfigured,
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontFamily: fontStack(font ?? ""),
      fontSize: resolvedFontSize,
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
          selectLinkAt(link, event);
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
      if (durationMilliseconds < longCommandNotifyMs || !document.hidden) return;
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
      select: selectLinkAt,
    });

    let decoder = new TextDecoder();
    let sent = "";
    const syncSize = () => {
      const stream = linkController.currentStream();
      const size = `${view.cols}x${view.rows}`;
      if (stream === null || view.cols === 0 || view.rows === 0 || size === sent) return;
      sent = size;
      stream.resize(view.cols, view.rows);
    };

    const linkController = createStreamLink({
      sessionId: session.id,
      api,
      onLink: setLink,
      onAttached: () => {
        sent = "";
        if (!coarse && session.state !== "exited" && !search.hasFocus()) {
          view.focus();
        }
        syncSize();
      },
      onReplay: (replay) => {
        if (!replay.truncated) return;
        decoder = new TextDecoder();
        // The retained ring may begin in the middle of CSI or OSC.
        // CAN makes xterm abandon that parser state without clearing
        // the visible screen as a full terminal reset would.
        view.write("\u0018");
        setTerminalNotice(t("terminal.replayTruncated"));
      },
      onOutput: (chunk) => {
        // A confirmation belongs to the exact terminal context the user
        // reviewed. Any output can be a new prompt or the reconnect
        // notice for a replacement shell, so require a fresh paste.
        setPendingPaste(null);
        view.write(decoder.decode(chunk, { stream: true }));
      },
      onExit: () => {
        view.options.cursorBlink = false;
        setPendingPaste(null);
        onExit?.();
      },
      onClose: () => setPendingPaste(null),
    });
    streamLinkController.current = linkController;

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
    fitAndSync();
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
    sendInput.current = (text) => linkController.currentStream()?.send(text);
    send.current = (name: string) => {
      const { ctrl, alt } = armed.current;
      const encoded = encodeKey(name, ctrl, alt);
      linkController.currentStream()?.send(encoded);
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    view.onData(typed);

    linkController.connect();

    sendPaste.current = (text) => linkController.currentStream()?.send(prepareTerminalPaste(text, view.modes.bracketedPasteMode));
    const detachClipboard = attachTerminalClipboard({
      container,
      terminal: view,
      paste: (text) => {
        const inspection = inspectTerminalPaste(text);
        if (inspection.requiresConfirmation) {
          setPendingPaste({ sessionId: session.id, raw: text });
          return;
        }
        sendPaste.current(text);
      },
      clipboard,
      coarsePointer: () => coarse,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
      enhancedKey: (event) => encodeIntlYen(event, intlYenRef.current) ?? kittyKeyboard.encode(event),
      sendEnhancedKey: (sequence) => linkController.currentStream()?.send(sequence),
    });

    const stopObservingSize = observeTerminalSize(view, container, fitAndSync);

    return () => {
      terminalDisposed = true;
      linkController.close();
      streamLinkController.current = null;
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
      fontSize: resolvedFontSize,
      scrollback: scrollbackLines,
    });
    if (changed.refit) refit.current?.();
  }, [resolvedFontSize, reducedMotion, scrollbackLines, session.state]);

  const connectionStatus = session.state === "connecting" || session.state === "reconnecting"
    ? connectionProgressText(t, session)
    : session.state === "connected"
      ? t("terminal.connected")
      : t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
  const displayTitle = terminalDisplayTitle(session);
  const subtitle = terminalSubtitle(session, t);
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
        onLinkNow={() => streamLinkController.current?.connect()}
        onLinkStop={() => streamLinkController.current?.stopRetrying()}
        {...(onStopReconnect === undefined ? {} : { onStopReconnect })}
        {...(onReconnect === undefined ? {} : { onReconnect })}
      />

      <div className="relative min-h-0 flex-1 overflow-clip bg-term-bg">
        <div
          ref={host}
          data-terminal-host=""
          {...keyboardOwnerProps}
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
      {pendingPaste === null || pendingPaste.sessionId !== session.id ? null : (
        <TerminalPasteDialog
          target={subtitle}
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
        onKey={(name) => send.current(name)}
      />
    </section>
  );
}

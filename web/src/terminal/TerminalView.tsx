import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { terminalSessionsApi, type TerminalSession, type TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { connectionProgressText } from "./progress";
import { exitedAbnormally, sessionShowsDestination, sessionStatusTone } from "./sessionStatus";
import { useTerminalFont } from "./useTerminalFont";
import { defaultTint } from "./appearance";
import { useBackgroundImage } from "./backgroundImage";
import { clipboard } from "../ui/clipboard";
import { attachImeKeys } from "./imeKeys";
import { attachSelectionOverlay, selectionHeldIn } from "./selectionOverlay";
import { linkAtPoint, type ScreenPoint } from "./tappedLink";
import { useTouchTerminal } from "./touchMode";
import { cellHeight, observeTerminalSize, syncTerminalInputPosition } from "./metrics";
import { attachTouchScroll } from "./touchScroll";
import { terminalTouchScrollTarget } from "./touchScrollTarget";
import { KeyBar, applyModifiers, encodeKey } from "./KeyBar";
import { useModifierKeys } from "./useModifierKeys";
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";
import { recentBufferText } from "./buffer";
import { attachOsc52Clipboard } from "./osc52";
import { createTerminalOutput } from "./output";
import type { ExitReport } from "./stream";
import { encodeIntlYen } from "./intlYenKey";
import { linkTextNamesTarget, modifierOpensLink, osc8Link } from "./links";
import { bufferRangeText } from "./bufferLinks";
import { attachLinkProvider } from "./linkProvider";
import { openTerminalURL, TerminalLinkPopover, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { TerminalQuickCommands } from "./TerminalQuickCommands";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";
import { TerminalPortForwards } from "./TerminalPortForwards";
import { attachWebglWhileShown } from "./webgl";
import { attachOSC7Directory } from "./osc7";
import { attachOSC9Directory } from "./osc9Directory";
import { attachOSC9Progress, noTerminalProgress, type TerminalProgress } from "./osc9Progress";
import { TerminalProgressBar } from "./TerminalProgressBar";
import { attachInlineImages } from "./inlineImages";
import { applyCurrentUnicodeWidths } from "./unicodeWidths";
import { attachCommandMarkers } from "./commandMarkers";
import { showBrowserNotification } from "../ui/browserNotifications";
import { isUserAway } from "../ui/userAway";
import { longCommandNotificationTag } from "./notificationTags";
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
import { TerminalComposer } from "./TerminalComposer";
import { planComposerSend } from "./composerSend";
import { pathPreviewAlias } from "./pathPreviewAlias";
import { useSameDestination } from "./useSameDestination";
import { previewPathsInBuffer, previewPathTargetsInBuffer } from "./previewPaths";
import { isHomePath, resolveTerminalPath } from "./relativePaths";
import { useLocalWindowsPaths } from "./useLocalWindowsPaths";
import { pathPreviewHome } from "./pathPreviewHome";
import { FilePreviewDialog } from "../sftp/FilePreviewDialog";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { useToast } from "../ui/useToast";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { cursorAnimationEnabled, reducedMotionQuery } from "../ui/reducedMotion";
import { ToastMessage } from "../ui/ToastMessage";
import { toastVisibleMs } from "../ui/ToastProvider";
import type { PaneEdge } from "./splitView/layout";
import { isTypingElsewhere } from "./terminalFocus";

type TerminalViewProps = {
  session: TerminalSession;
  searchShortcutActive?: boolean;
  active?: boolean;
  focusRequest?: number;
  headerActions?: ReactNode;
  onSplit?: (edge: PaneEdge) => void;
  canSplit?: boolean;
  api?: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
  // onExit hands over how the program ended, as the stream reported it.
  onExit?: (exit: ExitReport) => void;
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
  // onOpenNavigation opens the sidebar from the phone's top bar, which replaces the app header there.
  onOpenNavigation?: (() => void) | undefined;
  osc52Enabled?: boolean;
  // vpnProfile は、この接続が通るVPN経路の名前である。空なら通らない。
  vpnProfile?: string;
  // runTarget is the SSH alias of the engine this session runs on, when it is
  // not this machine. Port forwarding, Quick Commands and opening remote paths
  // in SFTP act on this machine's engine, so they are not offered for it.
  runTarget?: string;
  scrollbackLines?: number;
  onOsc52Change?: (enabled: boolean) => void | Promise<void>;
  onForwardsChanged?: () => void | Promise<void>;
  jisYenBackslash?: boolean;
  // composerEnterSends: Enter in the phone's text box sends; otherwise it starts a new line.
  composerEnterSends?: boolean;
};

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
  active = true,
  focusRequest,
  headerActions,
  onSplit,
  canSplit = false,
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
  onOpenNavigation,
  osc52Enabled: initialOsc52Enabled = false,
  vpnProfile = "",
  runTarget,
  scrollbackLines = defaultBrowserScrollbackLines,
  onOsc52Change,
  onForwardsChanged,
  jisYenBackslash = false,
  composerEnterSends = false,
}: TerminalViewProps) {
  const t = useTranslate();
  const showToast = useToast();
  const { resolved } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const mobile = useMediaQuery(mobileViewportQuery);
  const touchTerminal = useTouchTerminal();
  const touchTerminalRef = useRef(touchTerminal);
  touchTerminalRef.current = touchTerminal;
  const resolvedFontSize = fontSize ?? (mobile ? mobileTerminalFontSize : desktopTerminalFontSize);
  const resolvedFontFamily = useTerminalFont(font ?? "", resolvedFontSize);
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
  const activeRef = useRef(active);
  activeRef.current = active;
  const search = useTerminalSearch({
    shortcutActive: searchShortcutActive,
    region,
    // A touch screen would raise its keyboard again for the terminal.
    returnFocus: () => {
      if (!touchTerminalRef.current) terminal.current?.focus();
    },
  });
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
    // The text came from the phone's text box: Enter follows it and the box is
    // cleared once it is sent.
    fromComposer?: boolean;
  } | null>(null);
  const [composerText, setComposerText] = useState("");
  const sendPaste = useRef<(text: string) => void>(() => {});
  const allowWindowsPaths = useLocalWindowsPaths(session.kind, runTarget);
  const windowsPathsRef = useRef(allowWindowsPaths);
  windowsPathsRef.current = allowWindowsPaths;
  const [currentDirectory, setCurrentDirectory] = useState("");
  const [programProgress, setProgramProgress] = useState<TerminalProgress>(noTerminalProgress);

  const [quickCommandsOpen, setQuickCommandsOpen] = useState(false);
  const [quickCommandSelection, setQuickCommandSelection] = useState("");
  const [overflowOpen, setOverflowOpen] = useState(false);
  const overflowTrigger = useRef<HTMLButtonElement>(null);
  const [portForwardsOpen, setPortForwardsOpen] = useState(false);
  const [linkSelection, setLinkSelection] = useState<TerminalLinkSelection | null>(null);
  const [pathPreview, setPathPreview] = useState<{ path: string; paths: string[] } | null>(null);
  // previewLinkedPath opens a clicked file path's preview at once; null where this session's files cannot be read.
  const previewLinkedPath = useRef<((path: string) => void) | null>(null);
  const streamLinkController = useRef<StreamLinkController | null>(null);

  const { modifiers, tap: tapModifier, takeModifiers } = useModifierKeys();
  const send = useRef<(name: string) => void>(() => {});
  const sendInput = useRef<(text: string) => void>(() => {});

  useEffect(() => {
    setOsc52Enabled(initialOsc52Enabled);
    setCurrentDirectory("");
    setProgramProgress(noTerminalProgress);
    setPendingPaste(null);
  }, [initialOsc52Enabled, session.id, session.state]);

  useEffect(() => {
    if (terminalNotice === "") return;
    const timer = window.setTimeout(() => setTerminalNotice(""), toastVisibleMs);
    return () => window.clearTimeout(timer);
  }, [terminalNotice]);

  // A file path opens its preview without a menu first; what else the file offers is in the preview's
  // "…". A URL, and a path whose file cannot be read here, open the menu.
  const selectLinkAt = useCallback((link: TerminalLinkSelection["link"], point: ScreenPoint) => {
    const preview = previewLinkedPath.current;
    if (link.kind !== "url" && preview !== null) {
      preview(link.target);
      return;
    }
    setLinkSelection({ link, x: point.clientX + linkPopoverOffsetPx, y: point.clientY + linkPopoverOffsetPx });
  }, []);

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    let view: Terminal;
    view = new Terminal({
      allowProposedApi: true,
      allowTransparency: backgroundConfigured,
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontFamily: resolvedFontFamily,
      fontSize: resolvedFontSize,
      theme: terminalTheme(container, hasBackground),
      scrollback: scrollbackLines,
      // Clearing the screen at a prompt (Ctrl+L in bash and zsh) moves what
      // was on it into the scrollback instead of erasing it, as VTE, Windows
      // Terminal and Ghostty do. `clear` still empties the scrollback (CSI 3 J).
      scrollOnEraseInDisplay: true,
      // A drag goes to a program that reads the mouse (Claude Code, vim, tmux).
      // xterm.js lets Shift take the drag back for selecting, except on macOS,
      // where only Option does and only with this on.
      macOptionClickForcesSelection: true,
      // Programs ask whether the kitty keyboard protocol is there (CSI ? u)
      // before they turn it on; xterm.js answers and encodes the keys itself.
      vtExtensions: { kittyKeyboard: true },
      linkHandler: {
        activate: (event, target, range) => {
          const link = osc8Link(target, bufferRangeText(view.buffer.active, range), range.start.x - 1, range.end.x);
          if (link === null) return;
          if (modifierOpensLink(event) && linkTextNamesTarget(link)) {
            openTerminalURL(link.target);
            return;
          }
          selectLinkAt(link, event);
        },
      },
    });
    applyCurrentUnicodeWidths(view);
    const fit = new FitAddon();
    const searchAddon = new SearchAddon({ highlightLimit: 1000 });
    view.loadAddon(fit);
    view.loadAddon(searchAddon);
    view.open(container);
    const inlineImages = attachInlineImages(view);
    const renderer = attachWebglWhileShown(view, {
      enabled: webglEnabled,
      backgroundImage: backgroundConfigured,
    });
    const measured = () => container.clientWidth !== 0 && container.clientHeight !== 0;
    renderer.shown(measured());
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

    const output = createTerminalOutput(view);
    const detachOsc52 = attachOsc52Clipboard({
      parser: view.parser,
      enabled: () => osc52EnabledRef.current && !output.isReplaying(),
      writeText: (text) => clipboard.writeText(text),
      copied: () => setTerminalNotice(t("terminal.osc52Copied")),
      refused: () => setProblem(t("terminal.clipboardRefused")),
    });
    const osc7Directory = attachOSC7Directory(view.parser, setCurrentDirectory);
    const osc9Directory = attachOSC9Directory(view.parser, setCurrentDirectory);
    const osc9Progress = attachOSC9Progress(view.parser, setProgramProgress);
    const commandMarkers = attachCommandMarkers(view, ({ durationMilliseconds }) => {
      // A program that quit without clearing its OSC 9;4 progress leaves no
      // bar behind once the shell reports the command finished.
      setProgramProgress(noTerminalProgress);
      if (durationMilliseconds < longCommandNotifyMs || !isUserAway()) return;
      showBrowserNotification({
        title: "sshc",
        body: t("terminal.longCommandCompleted", {
          subject: terminalDisplayTitle(session),
          seconds: String(Math.round(durationMilliseconds / 1000)),
        }),
        tag: longCommandNotificationTag(session.id),
      });
    });
    const terminalLinks = attachLinkProvider(view, {
      open: openTerminalURL,
      select: selectLinkAt,
      allowWindowsPaths: () => windowsPathsRef.current,
    });

    let sent = "";
    const syncSize = () => {
      // Restoring a hidden session must preserve its PTY size until this
      // host can be measured, rather than sending xterm's initial 80x24.
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
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
        if (activeRef.current && !touchTerminalRef.current && session.state !== "exited" && !search.hasFocus() && !isTypingElsewhere(region.current)) {
          view.focus();
        }
        fitAndSync();
      },
      onReplay: (replay, resumed) => {
        output.beginReplay(replay);
        if (!replay.truncated) return;
        // Only a link that was offline lost output; a view shown afresh lacks
        // only the session's oldest output.
        setTerminalNotice(t(resumed ? "terminal.replayTruncated" : "terminal.replayTruncatedOldest"));
      },
      // Output alone keeps a pending paste: full-screen programs redraw all
      // the time, and on the focus change that opening the review causes.
      onOutput: output.write,
      // The paste was reviewed for the shell that is now gone.
      onShellReplaced: () => setPendingPaste(null),
      onExit: (exit) => {
        view.options.cursorBlink = false;
        setPendingPaste(null);
        onExit?.(exit);
      },
      onClose: () => setPendingPaste(null),
    });
    streamLinkController.current = linkController;

    const fitAndSync = () => {
      renderer.shown(measured());
      if (!measured()) return;
      try {
        fit.fit();
        if (touchTerminalRef.current) syncTerminalInputPosition(view);
      } catch {
        return;
      }
      syncSize();
      view.refresh(0, Math.max(0, view.rows - 1));
    };
    // Keyboard/orientation changes must resize the PTY even while text is
    // selected; the selection overlay releases its handles when its shape changes.
    // observeTerminalSize makes the first fit, once the rest of the terminal is attached.
    refit.current = fitAndSync;

    const detachTouchScroll = attachTouchScroll(container, terminalTouchScrollTarget(view), () => cellHeight(view, container), {
      selectionHeld: () => selectionHeldIn(container),
      reducedMotion: () => reducedMotionRef.current,
    });

    const typed = (data: string) => {
      // xterm answers the queries in replayed history (DA1, cursor position,
      // kitty image checks) as if they were new. The program that asked has
      // long had its answer; this one would land in the shell as typed text.
      if (output.isReplaying()) return;
      const { ctrl, alt } = takeModifiers();
      sendInput.current(applyModifiers(data, ctrl, alt));
    };
    sendInput.current = (text) => linkController.currentStream()?.send(text);
    send.current = (name: string) => {
      const { ctrl, alt } = takeModifiers();
      linkController.currentStream()?.send(encodeKey(name, ctrl, alt));
    };
    view.onData(typed);
    // A program that turns on mouse reports without SGR (1006) gets them in the
    // default format, which xterm.js hands out here rather than through onData.
    view.onBinary((data) => linkController.currentStream()?.sendBinary(data));

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
      coarsePointer: () => touchTerminalRef.current,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
      copied: () => setTerminalNotice(t("copy.done")),
      enhancedKey: (event) => encodeIntlYen(event, intlYenRef.current),
      sendEnhancedKey: (sequence) => linkController.currentStream()?.send(sequence),
      selectionIsSearchMatch: search.isSelectingMatch,
    });

    const stopObservingSize = observeTerminalSize(view, container, fitAndSync);

    return () => {
      linkController.close();
      streamLinkController.current = null;
      stopObservingSize();
      detachTouchScroll();
      detachClipboard();
      detachOsc52();
      output.dispose();
      osc7Directory.dispose();
      osc9Directory.dispose();
      osc9Progress.dispose();
      inlineImages.dispose();
      commandMarkers.dispose();
      terminalLinks.dispose();
      renderer.dispose();
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

  // Declared after the effect that builds the terminal, and keyed on the same
  // values, so it attaches to each new terminal. Switching between touch and
  // mouse changes only these, not the connection.
  useEffect(() => {
    const container = host.current;
    const view = terminal.current;
    if (!touchTerminal || container === null || view === null) return;
    const releaseImeKeys = attachImeKeys({ container, textarea: view.textarea ?? container });
    const detachOverlay = attachSelectionOverlay(container, view, {
      onTap: (point) => {
        const link = linkAtPoint(view, point, windowsPathsRef.current);
        if (link === null) return false;
        selectLinkAt(link, point);
        return true;
      },
    });
    refit.current?.();
    return () => {
      releaseImeKeys();
      detachOverlay();
    };
  }, [touchTerminal, selectLinkAt, session.id, api, backgroundConfigured, webglEnabled]);

  useEffect(() => {
    if (focusRequest !== undefined && activeRef.current && !region.current?.contains(document.activeElement)) terminal.current?.focus();
  }, [focusRequest]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = resolvedFontFamily;
    refit.current?.();
  }, [resolvedFontFamily]);

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
  const sameDestination = useSameDestination(remoteAlias, runTarget);
  const previewAlias = pathPreviewAlias(session, runTarget, sameDestination);

  // A relative path is relative to where the program that printed it works.
  // The engine reads that for a local shell; an SSH session only has the
  // directory its shell reported through OSC 7.
  async function previewPath(path: string) {
    if (previewAlias === undefined) return;
    const buffer = terminal.current?.buffer.active;
    const needsHome = isHomePath(path) || (buffer !== undefined && previewPathTargetsInBuffer(buffer, allowWindowsPaths).some(isHomePath));
    const [engineDirectory, home] = await Promise.all([
      api.terminalSessionDirectory(session.id).catch(() => null),
      needsHome ? pathPreviewHome(previewAlias).catch(() => null) : null,
    ]);
    const directory = engineDirectory ?? (currentDirectory === "" ? null : currentDirectory);
    const resolvedPath = resolveTerminalPath(path, directory, home);
    if (resolvedPath === null) {
      setTerminalNotice(t(isHomePath(path) ? "terminal.homePathUnknown" : "terminal.relativePathUnknown"));
      return;
    }
    setPathPreview({
      path: resolvedPath,
      paths: terminal.current === null ? [] : previewPathsInBuffer(terminal.current.buffer.active, { directory, home, allowWindowsPaths }),
    });
  }
  previewLinkedPath.current = previewAlias === undefined ? null : (path) => void previewPath(path);

  // SFTP can open a file of an SSH session at its host. A session opened on a run target reached its host
  // through that engine's SSH config, which this machine may not share; only the same destination will do.
  const openInSFTP = (runTarget === undefined || sameDestination) && remoteAlias !== undefined && onOpenRemotePath !== undefined
    ? (path: string, action: RemotePathAction) => {
      setPathPreview(null);
      onOpenRemotePath(remoteAlias, path, action);
    }
    : undefined;
  function previewedFileActions(path: string): ActionMenuItem[] {
    return [
      ...(openInSFTP === undefined ? [] : [
        { label: t("terminal.linkBrowseSFTP"), onSelect: () => openInSFTP(path, "browse") },
        { label: t("terminal.linkEditSFTP"), onSelect: () => openInSFTP(path, "edit") },
        { label: t("terminal.linkDownloadSFTP"), onSelect: () => openInSFTP(path, "download") },
      ]),
      {
        label: t("sftp.copyPath"),
        onSelect: () => void clipboard.writeText(path)
          .then(() => showToast(t("copy.done"), { tone: "success" }))
          .catch(() => showToast(t("copy.refused"))),
      },
    ];
  }

  function submitComposer() {
    const plan = planComposerSend(composerText, { bracketedPaste: terminal.current?.modes.bracketedPasteMode ?? false });
    if (plan.kind === "confirm") {
      setPendingPaste({ sessionId: session.id, raw: plan.text, fromComposer: true });
      return;
    }
    if (plan.kind === "paste") sendPaste.current(plan.text);
    sendInput.current("\r");
    setComposerText("");
  }

  return (
    <section ref={region} aria-label={t("terminal.screenLabel", { title: displayTitle })} className="relative flex min-h-0 flex-1 flex-col">
      {/* The header follows the design: a 40px row on a desktop, the screen's top bar on a phone
          (‹ opens the navigation, the title and destination, and the other actions). */}
      <div className={`group/header relative flex shrink-0 items-center ${mobile ? "h-[54px] gap-1.5 pl-0.5 pr-1.5" : "h-10 gap-[9px] bg-term-bg pl-3.5 pr-2.5"}`}>
        {mobile && onOpenNavigation !== undefined ? (
          <button
            type="button"
            aria-label={t("shell.navigationToggle")}
            title={t("shell.navigationToggle")}
            onClick={onOpenNavigation}
            className="flex size-11 shrink-0 items-center justify-center rounded-lg text-ink-muted hover:bg-hover"
          >
            <Icon name="chevronLeft" className="size-[22px]" />
          </button>
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-[9px]">
            <span
              aria-hidden="true"
              className={`size-2 shrink-0 rounded-full ${
                session.state === "exited"
                  ? exitedAbnormally(session) ? "bg-danger" : "bg-ink-faint"
                  : session.state === "reconnecting" || session.state === "connecting" || link.phase !== "live"
                    ? "bg-notice-ink"
                    : "bg-live"
              }`}
            />
            <p className={`min-w-0 truncate font-semibold text-ink ${mobile ? "text-[15px]" : "max-w-48 shrink-0 text-[13px]"}`}>{displayTitle}</p>
            {mobile ? null : <span className="min-w-0 truncate font-mono text-[11.5px] text-ink-faint">{subtitle}</span>}
            {/* While connected the dot says it; the words stay for screen readers. A phone has no room for them
                either way, but they are still read out there. */}
            <span role="status" className={(sessionShowsDestination(session) && link.phase === "live") || mobile ? "sr-only" : `shrink-0 text-[11.5px] ${sessionStatusTone(session)}`}>{connectionStatus}</span>
            {mobile ? null : <VPNProfileChip name={vpnProfile} />}
            {mobile || runTarget === undefined ? null : (
              <span className="shrink-0 rounded-full bg-accent/15 px-2 py-px font-mono text-[11px] text-accent">{t("runTarget.runningOn", { alias: runTarget })}</span>
            )}
          </div>
          {mobile ? (
            <p className="mt-px flex min-w-0 items-center gap-1 pl-[17px] text-xs text-ink-faint">
              <span className="truncate">{subtitle}</span>
              {sessionShowsDestination(session) && link.phase === "live" ? null : <span className={`shrink-0 ${sessionStatusTone(session)}`}>· {connectionStatus}</span>}
            </p>
          ) : null}
        </div>
        {mobile ? null : (
          <button
            type="button"
            aria-label={t("terminal.search")}
            title={t("terminal.search")}
            className="flex size-7 shrink-0 items-center justify-center rounded-[7px] text-ink-faint hover:bg-hover hover:text-ink focus:bg-hover focus:outline-none"
            onClick={search.toggle}
          >
            <Icon name="search" className="size-[15px]" />
          </button>
        )}
        <button
          ref={overflowTrigger}
          type="button"
          aria-label={t("terminal.moreActions")}
          aria-expanded={overflowOpen}
          className={`flex shrink-0 items-center justify-center text-ink-faint hover:bg-hover hover:text-ink focus:bg-hover focus:outline-none ${mobile ? "size-11 rounded-lg" : "size-7 rounded-[7px]"}`}
          onClick={() => setOverflowOpen((current) => !current)}
        >
          <Icon name="moreHorizontal" className={mobile ? "size-5" : "size-4"} />
        </button>
        {headerActions}
        {overflowOpen ? (
          <TerminalOverflowMenu
            {...(mobile ? { onSearch: search.toggle } : {})}
            onSplit={onSplit}
            canSplit={canSplit}
            triggerRef={overflowTrigger}
            osc52Enabled={osc52Enabled}
            onQuickCommands={runTarget !== undefined ? undefined : () => {
              setQuickCommandSelection(terminal.current?.getSelection() ?? "");
              setQuickCommandsOpen(true);
            }}
            onPortForwarding={session.kind === "ssh" && runTarget === undefined ? () => setPortForwardsOpen(true) : undefined}
            onOpenRemoteDirectory={runTarget === undefined && session.kind === "ssh" && session.alias !== undefined && currentDirectory !== "" && onOpenRemotePath !== undefined
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
        <TerminalProgressBar progress={programProgress} />
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

      <div className={`relative min-h-0 flex-1 overflow-clip bg-term-bg ${mobile ? "mx-2 rounded-[14px] border border-line" : ""}`}>
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
          <ToastMessage role="status" className="absolute bottom-3 right-3 z-20 max-w-[min(24rem,calc(100%-1.5rem))]">
            {terminalNotice}
          </ToastMessage>
        )}
      </div>
      {quickCommandsOpen ? (
        <TerminalQuickCommands
          session={session}
          initialCommand={quickCommandSelection}
          returnFocusRef={overflowTrigger}
          onClose={() => setQuickCommandsOpen(false)}
          onCopied={() => setTerminalNotice(t("copy.done"))}
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
        />
      )}
      {previewAlias === undefined || pathPreview === null ? null : (
        <FilePreviewDialog
          alias={previewAlias}
          path={pathPreview.path}
          paths={pathPreview.paths}
          actions={previewedFileActions}
          onClose={() => setPathPreview(null)}
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
            if (pendingPaste.fromComposer === true) {
              sendInput.current("\r");
              setComposerText("");
            }
          }}
        />
      )}
      <KeyBar
        modifiers={modifiers}
        onTap={tapModifier}
        onKey={(name) => send.current(name)}
      />
      <TerminalComposer
        value={composerText}
        disabled={link.phase !== "live"}
        enterSends={composerEnterSends}
        onChange={setComposerText}
        onSubmit={submitComposer}
      />
    </section>
  );
}

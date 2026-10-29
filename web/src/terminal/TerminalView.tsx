import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { Terminal } from "@xterm/xterm";
import { terminalSessionsApi, type TerminalSession, type TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { exitedAbnormally, sessionStateText, sessionStatusTone } from "./sessionStatus";
import { useTerminalFont } from "./useTerminalFont";
import { defaultTint } from "./appearance";
import { useBackgroundImage } from "./backgroundImage";
import { clipboard } from "../ui/clipboard";
import { attachImeKeys } from "./imeKeys";
import { attachSelectionOverlay, selectionHeldIn } from "./selectionOverlay";
import { cellAtPoint, linkAtPoint, type ScreenPoint } from "./tappedLink";
import { attachPromptCursorClicks, type PromptCursor } from "./promptCursor";
import { attachPromptJumpKeys, type PromptJump } from "./promptJump";
import { useTouchTerminal } from "./touchMode";
import { cellHeight, observeTerminalSize, syncTerminalInputPosition } from "./metrics";
import { attachFontZoom } from "./attachFontZoom";
import { attachTouchScroll } from "./touchScroll";
import { terminalTouchScrollTarget } from "./touchScrollTarget";
import { KeyBar, applyModifiers, encodeKey } from "./KeyBar";
import { useModifierKeys } from "./useModifierKeys";
import type { ModifierName } from "./modifierLatch";
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";
import { createTerminalOutput } from "./output";
import { createStatusLineRouter } from "./statusLine";
import { createStatusLineTerminal, type StatusLineTerminal } from "./statusLineTerminal";
import type { ExitReport } from "./stream";
import { encodeIntlYen } from "./intlYenKey";
import { attachLinkProvider } from "./linkProvider";
import { openTerminalURL, TerminalLinkPopover, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { TerminalQuickCommands } from "./TerminalQuickCommands";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";
import { TerminalPortForwards } from "./TerminalPortForwards";
import { noTerminalProgress, type TerminalProgress } from "./osc9Progress";
import { TerminalProgressBar } from "./TerminalProgressBar";
import type { LigatureDrawing } from "./ligatures";
import { createTerminalView } from "./createTerminalView";
import { terminalCopyActions } from "./terminalCopyActions";
import { attachTerminalSequences } from "./attachTerminalSequences";
import type { CommandBlock } from "./commandBlocks";
import { attachCommandDisplay, type CommandDisplay } from "./commandDisplay";
import { TerminalCommandCard } from "./TerminalCommandCard";
import { TerminalCommandHeader } from "./TerminalCommandHeader";
import { useCommandCard } from "./useCommandCard";
import { useTerminalPathPreview } from "./useTerminalPathPreview";
import { isTerminalShown, watchTerminalShown } from "./terminalVisibility";
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
import { terminalCommandApiFor } from "./commands/commandApi";
import { forwardApiFor } from "./forwardApi";
import { useLocalWindowsPaths } from "./useLocalWindowsPaths";
import { FilePreviewDialog } from "../sftp/FilePreviewDialog";
import type { RemotePathAction } from "../sftp/SFTPPanel";
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
  // onRemoveFromSplit takes this session out of the split it is shown in.
  onRemoveFromSplit?: () => void;
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
  // not this machine. Quick Commands and port forwarding then go to that
  // engine; opening a path in SFTP is offered only when this machine reaches
  // the same destination.
  runTarget?: string;
  scrollbackLines?: number;
  onOsc52Change?: (enabled: boolean) => void | Promise<void>;
  onForwardsChanged?: () => void | Promise<void>;
  jisYenBackslash?: boolean;
  // composerEnterSends: Enter in the phone's text box sends; otherwise it starts a new line.
  composerEnterSends?: boolean;
  // ligatures draws sequences such as -> and != with the font's ligatures.
  ligatures?: boolean;
  // statusLine shows the VT320 status line a program shows (statusLine.ts). When off, what is written
  // to it stays off the screen too.
  statusLine?: boolean;
  // With shell integration (OSC 133): the marks left of each command (commandGutter.ts), the command kept
  // above its output while that is scrolled through, and the marks on the scrollbar.
  commandMarks?: boolean;
  stickyCommand?: boolean;
  commandScrollbarMarks?: boolean;
};

// The status line takes exactly one row from the screen. Its padding above and below its row (index.css)
// lies over the screen's own padding under the last row, which holds no text.
const statusLinePadding = "0.5rem";

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
  onRemoveFromSplit,
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
  ligatures = false,
  statusLine: statusLineEnabled = true,
  commandMarks = true,
  stickyCommand = true,
  commandScrollbarMarks = true,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const reducedMotion = useMediaQuery(reducedMotionQuery);
  const mobile = useMediaQuery(mobileViewportQuery);
  const touchTerminal = useTouchTerminal();
  const touchTerminalRef = useRef(touchTerminal);
  touchTerminalRef.current = touchTerminal;
  const resolvedFontSize = fontSize ?? (mobile ? mobileTerminalFontSize : desktopTerminalFontSize);
  // zoomedFontSize is the size a pinch, or the wheel with Ctrl, gave this terminal over the settings' size
  // (base). It is this view's alone and is not saved. When the settings' size changes, the view follows it again.
  const [zoomedFontSize, setZoomedFontSize] = useState<{ base: number; size: number } | null>(null);
  const shownFontSize = zoomedFontSize !== null && zoomedFontSize.base === resolvedFontSize ? zoomedFontSize.size : resolvedFontSize;
  const shownFontSizeRef = useRef(shownFontSize);
  shownFontSizeRef.current = shownFontSize;
  const resolvedFontSizeRef = useRef(resolvedFontSize);
  resolvedFontSizeRef.current = resolvedFontSize;
  const resolvedFontFamily = useTerminalFont(font ?? "", shownFontSize);
  const reducedMotionRef = useRef(reducedMotion);
  reducedMotionRef.current = reducedMotion;
  const host = useRef<HTMLDivElement>(null);
  const statusLineHost = useRef<HTMLDivElement>(null);
  const statusLine = useRef<StatusLineTerminal | null>(null);
  // shownStatusLineHeight is the row height of the VT320 status line a program shows (statusLine.ts), or null.
  // The line is drawn under the screen only with the setting on; it is written to either way.
  const [shownStatusLineHeight, setStatusLineHeight] = useState<number | null>(null);
  const statusLineHeight = statusLineEnabled ? shownStatusLineHeight : null;
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
  // drawnLive tells whether the screen is drawn as it is while the link is live, without the banner
  // above it that says the link is connecting (see the layout effect below).
  const drawnLive = useRef(false);
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
  const copyWithColors = useRef<() => void>(() => {});
  const saveScrollback = useRef<() => void>(() => {});
  const commandOutput = useRef<{ copy: (block: CommandBlock) => void; save: (block: CommandBlock) => void } | null>(null);
  const commandDisplay = useRef<CommandDisplay | null>(null);
  const commandMarksRef = useRef(commandMarks);
  commandMarksRef.current = commandMarks;
  const commandScrollbarMarksRef = useRef(commandScrollbarMarks);
  commandScrollbarMarksRef.current = commandScrollbarMarks;
  const commandCard = useCommandCard();
  // topCommand is the command whose output fills the top of the view while its prompt is above it.
  const [topCommand, setTopCommand] = useState<CommandBlock | null>(null);
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
  const ligaturesRef = useRef(ligatures);
  ligaturesRef.current = ligatures;
  const ligatureDrawing = useRef<LigatureDrawing | null>(null);
  const [currentDirectory, setCurrentDirectory] = useState("");
  const [programProgress, setProgramProgress] = useState<TerminalProgress>(noTerminalProgress);

  const [quickCommandsOpen, setQuickCommandsOpen] = useState(false);
  const [quickCommandSelection, setQuickCommandSelection] = useState("");
  const [overflowOpen, setOverflowOpen] = useState(false);
  const overflowTrigger = useRef<HTMLButtonElement>(null);
  const [portForwardsOpen, setPortForwardsOpen] = useState(false);
  const [linkSelection, setLinkSelection] = useState<TerminalLinkSelection | null>(null);
  const pathPreview = useTerminalPathPreview({
    session,
    runTarget,
    api,
    terminal,
    currentDirectory,
    allowWindowsPaths,
    onOpenRemotePath,
    notice: setTerminalNotice,
  });
  const previewLinkedPath = pathPreview.previewLinkedPath;
  const streamLinkController = useRef<StreamLinkController | null>(null);

  const { modifiers, tap: tapModifier, takeModifiers } = useModifierKeys();
  // The phone's text box, while it is shown (TerminalComposer).
  const composerField = useRef<HTMLTextAreaElement>(null);
  const send = useRef<(name: string) => void>(() => {});
  const sendInput = useRef<(text: string) => void>(() => {});
  const promptCursor = useRef<PromptCursor | null>(null);
  const promptJump = useRef<PromptJump | null>(null);

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
  }, [previewLinkedPath]);

  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const parts = createTerminalView(container, {
      backgroundImage: backgroundConfigured,
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontFamily: resolvedFontFamily,
      fontSize: shownFontSize,
      theme: terminalTheme(container, hasBackground),
      scrollback: scrollbackLines,
      webgl: webglEnabled,
      webglSettled: () => refit.current?.(),
      ligatures: ligaturesRef.current,
      selectLink: selectLinkAt,
    });
    const { view, fit, searchAddon, serializeAddon, renderer } = parts;
    ligatureDrawing.current = parts.ligatureDrawing;
    const measured = () => container.clientWidth !== 0 && container.clientHeight !== 0;
    renderer.shown(measured());
    terminal.current = view;

    const unbindSearch = search.bind(view, searchAddon, container);
    const copyActions = terminalCopyActions({
      view,
      serializeAddon,
      title: () => terminalDisplayTitle(session),
      t,
      notice: setTerminalNotice,
      problem: setProblem,
    });
    saveScrollback.current = copyActions.saveScrollback;
    copyWithColors.current = copyActions.copyWithColors;
    copyContext.current = copyActions.copyContext;
    commandOutput.current = { copy: copyActions.copyCommandOutput, save: copyActions.saveCommandOutput };

    const statusLineTerminal = createStatusLineTerminal(view);
    statusLine.current = statusLineTerminal;
    // replaySizeHeld is true while the history is replayed at the size it was written for (output.ts): the
    // terminal neither fits its container nor tells the engine those sizes until the history is over.
    let replaySizeHeld = false;
    const output = createTerminalOutput(createStatusLineRouter({
      screen: view,
      statusLine: () => statusLineTerminal,
      shownChanged: (shown) => {
        if (shown) statusLineTerminal.clear();
        setStatusLineHeight(shown ? cellHeight(view, container) : null);
      },
    }), {
      replayAt: (size) => {
        replaySizeHeld = true;
        if (view.cols !== size.cols || view.rows !== size.rows) view.resize(size.cols, size.rows);
      },
      replayed: () => {
        replaySizeHeld = false;
        fitAndSync();
      },
    });
    const sequences = attachTerminalSequences({
      view,
      t,
      replaying: () => output.isReplaying(),
      clipboardWritesAllowed: () => osc52EnabledRef.current,
      reply: (sequence) => sendInput.current(sequence),
      notice: setTerminalNotice,
      problem: setProblem,
      setDirectory: setCurrentDirectory,
      setProgress: setProgramProgress,
    });
    const { kittyClipboard, prompt, prompts } = sequences;
    commandDisplay.current = attachCommandDisplay(view, sequences.commandBlocks, {
      container,
      marks: () => commandMarksRef.current,
      scrollbarMarks: () => commandScrollbarMarksRef.current,
      point: commandCard.point,
      leave: commandCard.leave,
      topChanged: setTopCommand,
    });
    promptJump.current = prompts;
    const detachPromptJumpKeys = attachPromptJumpKeys(container, prompts);
    promptCursor.current = prompt;
    const detachPromptClicks = attachPromptCursorClicks({
      container,
      prompt,
      cellAt: (point) => cellAtPoint(view, point),
      hasSelection: () => view.hasSelection(),
      enabled: () => !touchTerminalRef.current,
      send: (keys) => sendInput.current(keys),
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
      // While the WebGL renderer loads, the cells are the DOM renderer's, wider than the whole
      // pixels WebGL draws them in: the program would get fewer columns, then the right ones.
      // The view is fitted again once the renderer is settled.
      if (renderer.attaching()) return;
      if (!drawnLive.current) return;
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
        output.abandonReplay();
        // A new stream starts as shown to the engine; it is told the truth at once.
        linkController.currentStream()?.reportShown(isTerminalShown(container));
        if (activeRef.current && !touchTerminalRef.current && session.state !== "exited" && !search.hasFocus() && !isTypingElsewhere(region.current)) {
          view.focus();
        }
        fitAndSync();
      },
      onReplay: (replay, resumed) => {
        output.beginReplay(replay, { sized: !resumed });
        if (!replay.truncated) return;
        // Only a link that was offline lost output; a view shown afresh lacks
        // only the session's oldest output.
        setTerminalNotice(t(resumed ? "terminal.replayTruncated" : "terminal.replayTruncatedOldest"));
      },
      onSkipped: (screen) => {
        output.skip(screen);
        setTerminalNotice(t("terminal.outputSkipped"));
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
      onClose: () => {
        output.abandonReplay();
        setPendingPaste(null);
      },
    });
    streamLinkController.current = linkController;

    const fitAndSync = () => {
      renderer.shown(measured());
      if (!measured() || replaySizeHeld) return;
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
    const detachFontZoom = attachFontZoom(container, {
      fontSize: () => shownFontSizeRef.current,
      setFontSize: (size) => setZoomedFontSize({ base: resolvedFontSizeRef.current, size }),
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
      nothingToPaste: (image) => setTerminalNotice(t(image ? "terminal.pasteImageUnsupported" : "terminal.pasteEmpty")),
      takePaste: (items) => kittyClipboard.takePaste(items),
      copied: () => setTerminalNotice(t("copy.done")),
      enhancedKey: (event) => encodeIntlYen(event, intlYenRef.current),
      sendEnhancedKey: (sequence) => linkController.currentStream()?.send(sequence),
      selectionIsSearchMatch: search.isSelectingMatch,
    });

    const stopObservingSize = observeTerminalSize(view, container, fitAndSync);
    const stopWatchingShown = watchTerminalShown(container, (shown) => linkController.currentStream()?.reportShown(shown));

    return () => {
      linkController.close();
      streamLinkController.current = null;
      stopObservingSize();
      stopWatchingShown();
      detachTouchScroll();
      detachFontZoom();
      detachClipboard();
      output.dispose();
      statusLineTerminal.dispose();
      statusLine.current = null;
      setStatusLineHeight(null);
      detachPromptClicks();
      detachPromptJumpKeys();
      commandDisplay.current?.dispose();
      commandDisplay.current = null;
      commandOutput.current = null;
      commandCard.close();
      setTopCommand(null);
      sequences.dispose();
      promptJump.current = null;
      promptCursor.current = null;
      terminalLinks.dispose();
      unbindSearch();
      parts.dispose();
      ligatureDrawing.current = null;
      terminal.current = null;
      sendInput.current = () => {};
      sendPaste.current = () => {};
      copyContext.current = () => {};
      copyWithColors.current = () => {};
      saveScrollback.current = () => {};
      copyActions.release();
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
        if (link !== null) {
          selectLinkAt(link, point);
          return true;
        }
        // A tap on the line being typed also moves the cursor there; it still
        // raises the keyboard to type at the new place.
        const cell = cellAtPoint(view, point);
        const keys = cell === null ? null : promptCursor.current?.arrowsTo(cell) ?? null;
        if (keys !== null) sendInput.current(keys);
        return false;
      },
      // A phone types into its text box, where Japanese is composed before it is sent; the terminal
      // takes the keys typed with the key bar's Ctrl or Alt (holdModifier below).
      typingTarget: () => {
        const field = composerField.current;
        return field === null || field.disabled ? view : field;
      },
    });
    refit.current?.();
    return () => {
      releaseImeKeys();
      detachOverlay();
    };
  }, [touchTerminal, selectLinkAt, session.id, api, backgroundConfigured, webglEnabled]);

  // Showing a session (opening one, picking one, or the one left after closing another) focuses it. A touch
  // terminal is left alone: focusing it raises the software keyboard, which comes up only for a tap on the terminal.
  useEffect(() => {
    if (focusRequest !== undefined && activeRef.current && !touchTerminalRef.current && !region.current?.contains(document.activeElement)) terminal.current?.focus();
  }, [focusRequest]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  // The command marks follow their settings and the theme's colours.
  const { pointed: pointedCommand, close: closeCommandCard } = commandCard;
  useEffect(() => {
    commandDisplay.current?.refresh();
    if (!commandMarks) closeCommandCard();
  }, [commandMarks, commandScrollbarMarks, resolved, palette, closeCommandCard]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = resolvedFontFamily;
    refit.current?.();
  }, [resolvedFontFamily]);

  // A stream is attached as the link turns live, while the banner that says it is connecting still
  // takes a row or two above the screen. A size measured then would be followed by the right one
  // once the banner goes, and a shell or a full-screen program would redraw twice. A layout effect
  // runs after React has drawn the screen without the banner.
  useLayoutEffect(() => {
    drawnLive.current = link.phase === "live";
    if (drawnLive.current) refit.current?.();
  }, [link.phase]);

  useEffect(() => {
    ligatureDrawing.current?.set(ligatures);
  }, [ligatures]);

  useEffect(() => {
    if (statusLineHeight !== null && statusLineHost.current !== null) statusLine.current?.open(statusLineHost.current);
  }, [statusLineHeight]);

  useEffect(() => {
    if (terminal.current === null) return;
    const changed = applyTerminalRuntimeOptions(terminal.current.options, {
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontSize: shownFontSize,
      scrollback: scrollbackLines,
    });
    if (changed.refit) refit.current?.();
  }, [shownFontSize, reducedMotion, scrollbackLines, session.state]);

  const connectionStatus = sessionStateText(t, session);
  // The header puts the state in words only while it is connecting. Connected, the dot says it; reconnecting,
  // ended or with the browser's link down, the row under the header says it, and says it once. The words stay
  // for screen readers either way.
  const headerSaysState = session.state === "connecting" && link.phase === "live";
  const displayTitle = terminalDisplayTitle(session);
  const subtitle = terminalSubtitle(session, t);
  // Ctrl and Alt apply to the next key typed into the terminal, so one held while typing in the text box
  // moves the typing there, keeping the keyboard up. A closed keyboard stays closed: the key bar has already
  // let go of the focus then (keepKeyboardAsIs).
  function holdModifier(name: ModifierName) {
    if (modifiers[name] === "off" && document.activeElement === composerField.current) terminal.current?.focus();
    tapModifier(name);
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
            <span role="status" className={!headerSaysState || mobile ? "sr-only" : `shrink-0 text-[11.5px] ${sessionStatusTone(session)}`}>{connectionStatus}</span>
            {mobile ? null : <VPNProfileChip name={vpnProfile} />}
            {mobile || runTarget === undefined ? null : (
              <span className="shrink-0 rounded-full bg-accent/15 px-2 py-px font-mono text-[11px] text-accent">{t("runTarget.runningOn", { alias: runTarget })}</span>
            )}
          </div>
          {mobile ? (
            <p className="mt-px flex min-w-0 items-center gap-1 pl-[17px] text-xs text-ink-faint">
              <span className="truncate">{subtitle}</span>
              {headerSaysState ? <span className={`shrink-0 ${sessionStatusTone(session)}`}>· {connectionStatus}</span> : null}
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
            {...(mobile ? {
              onSearch: search.toggle,
              onPreviousPrompt: () => promptJump.current?.jump(-1),
              onNextPrompt: () => promptJump.current?.jump(1),
            } : {})}
            onSplit={onSplit}
            onRemoveFromSplit={onRemoveFromSplit}
            canSplit={canSplit}
            triggerRef={overflowTrigger}
            osc52Enabled={osc52Enabled}
            onQuickCommands={() => {
              setQuickCommandSelection(terminal.current?.getSelection() ?? "");
              setQuickCommandsOpen(true);
            }}
            onPortForwarding={session.kind === "ssh" ? () => setPortForwardsOpen(true) : undefined}
            onOpenRemoteDirectory={runTarget === undefined && session.kind === "ssh" && session.alias !== undefined && currentDirectory !== "" && onOpenRemotePath !== undefined
              ? () => onOpenRemotePath(session.alias!, currentDirectory, "browse")
              : undefined}
            onCopyContext={() => copyContext.current()}
            onCopyWithColors={() => copyWithColors.current()}
            onSaveScrollback={() => saveScrollback.current()}
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
          style={{
            ...(hasBackground
              ? {
                  "--ui-term-image": `url("${backgroundURL}")`,
                  "--ui-term-tint": String(tint ?? defaultTint),
                } as CSSProperties
              : {}),
            // The screen gives up a row to the status line; the size observer refits it.
            ...(statusLineHeight === null ? {} : { bottom: `${statusLineHeight}px` }),
          }}
          // Two fingers on the screen pinch its text (attachFontZoom), not the page.
          className="absolute inset-0 bg-term-bg [touch-action:pan-x_pan-y]"
        />
        <div
          ref={statusLineHost}
          data-terminal-status-line=""
          hidden={statusLineHeight === null}
          style={statusLineHeight === null ? undefined : { height: `calc(${statusLineHeight}px + ${statusLinePadding})` }}
          className="absolute inset-x-0 bottom-0 overflow-hidden"
        />
        {stickyCommand && topCommand !== null && !search.open ? (
          <TerminalCommandHeader block={topCommand} onGoTo={() => terminal.current?.scrollToLine(topCommand.start.line)} />
        ) : null}
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
          api={terminalCommandApiFor(runTarget)}
          onClose={() => setQuickCommandsOpen(false)}
          onCopied={() => setTerminalNotice(t("copy.done"))}
        />
      ) : null}
      {portForwardsOpen ? (
        <TerminalPortForwards
          session={session}
          api={forwardApiFor(runTarget)}
          isRunTargetSession={runTarget !== undefined}
          {...(onForwardsChanged === undefined ? {} : { onChanged: onForwardsChanged })}
          onClose={() => setPortForwardsOpen(false)}
        />
      ) : null}
      {commandMarks && pointedCommand !== null ? (
        <TerminalCommandCard
          block={pointedCommand.block}
          mark={pointedCommand.mark}
          onCopy={() => commandOutput.current?.copy(pointedCommand.block)}
          onSave={() => commandOutput.current?.save(pointedCommand.block)}
          onClose={commandCard.close}
          onPointerEnter={commandCard.keep}
          onPointerLeave={commandCard.leave}
        />
      ) : null}
      {linkSelection === null ? null : (
        <TerminalLinkPopover
          selection={linkSelection}
          onClose={() => setLinkSelection(null)}
        />
      )}
      {pathPreview.previewAlias === undefined || pathPreview.shown === null ? null : (
        <FilePreviewDialog
          alias={pathPreview.previewAlias}
          path={pathPreview.shown.path}
          paths={pathPreview.shown.paths}
          actions={pathPreview.fileActions}
          onClose={pathPreview.close}
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
        onTap={holdModifier}
        onKey={(name) => send.current(name)}
      />
      <TerminalComposer
        value={composerText}
        disabled={link.phase !== "live"}
        enterSends={composerEnterSends}
        fieldRef={composerField}
        onChange={setComposerText}
        onSubmit={submitComposer}
      />
    </section>
  );
}

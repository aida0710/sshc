import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { Terminal } from "@xterm/xterm";
import { terminalSessionsApi, type TerminalSession, type TerminalSessionsApi } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { reconnectsOnEnter } from "./sessionStatus";
import { useTerminalFont } from "./useTerminalFont";
import { defaultTint } from "./appearance";
import { useBackgroundImage } from "./backgroundImage";
import { attachImeKeys } from "./imeKeys";
import { attachSelectionOverlay } from "./selectionOverlay";
import { cellAtPoint, linkAtPoint, type ScreenPoint } from "./tappedLink";
import { useTouchTerminal } from "./touchMode";
import { flushSync } from "react-dom";
import { KeyBar } from "./KeyBar";
import { useKeyBarLayout } from "../keybar/keyBarPresets";
import { offersDirectInput } from "../keybar/keyBarLayout";
import { setDirectInput, useDirectInput } from "./useDirectInput";
import { useModifierKeys } from "./useModifierKeys";
import type { ModifierName } from "./modifierLatch";
import { terminalDisplayTitle, terminalSubtitle } from "./terminalPresentation";
import type { ExitReport } from "./stream";
import { useTouchLinkUnderlines } from "./touchLinkUnderlines";
import { TerminalLinkPopover, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { TerminalQuickCommands } from "./TerminalQuickCommands";
import { TerminalOverflowMenu } from "./TerminalOverflowMenu";
import { TerminalPortForwards } from "./TerminalPortForwards";
import { useProgramProgress } from "./useProgramProgress";
import { TerminalProgressBar } from "./TerminalProgressBar";
import { TerminalHeader } from "./TerminalHeader";
import { assembleTerminal, type AssembledTerminal, type TerminalLiveValues } from "./terminalAssembly";
import type { CommandBlock } from "./commandBlocks";
import { TerminalCommandCard } from "./TerminalCommandCard";
import { TerminalCommandHeader } from "./TerminalCommandHeader";
import { useCommandCard } from "./useCommandCard";
import { useTerminalPathPreview } from "./useTerminalPathPreview";
import { applyTerminalRuntimeOptions } from "./runtimeOptions";
import { Icon } from "../ui/icons";
import { isDismissibleLayerOpen, keyboardOwnerProps } from "../ui/useDismissibleLayer";
import { useAndroidBackStop } from "../ui/useAndroidBackStop";
import { useTerminalSearch } from "./useTerminalSearch";
import { TerminalSearchBar } from "./TerminalSearchBar";
import { TerminalStatusBanners } from "./TerminalStatusBanners";
import { TerminalNoticeCard } from "./TerminalNoticeCard";
import { TerminalNoticesButton } from "./TerminalNoticesButton";
import { useTerminalNotices } from "./useTerminalNotices";
import { savedConnectionOf } from "./terminalPresentation";
import { useSessionRecovery } from "./useSessionRecovery";
import { useHostKeyAnswer } from "../shell/useHostKeyAnswer";
import { type StreamLink } from "./streamLink";
import { defaultBrowserScrollbackLines } from "./browserScrollback";
import { TerminalPasteDialog } from "./TerminalPasteDialog";
import { TerminalComposer } from "./TerminalComposer";
import { useTerminalComposer } from "./useTerminalComposer";
import { terminalCommandApiFor } from "./commands/commandApi";
import { forwardApiFor } from "./forwardApi";
import { useLocalWindowsPaths } from "./useLocalWindowsPaths";
import { FilePreviewDialog } from "../sftp/FilePreviewDialog";
import type { RemotePathAction } from "../sftp/SFTPPanel";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { cursorAnimationEnabled } from "../ui/reducedMotion";
import { useReducedMotion } from "../theme/motionContext";
import { ToastMessage } from "../ui/ToastMessage";
import { toastVisibleMs } from "../ui/ToastProvider";
import type { PaneEdge } from "./splitView/layout";
import { isTypingElsewhere } from "./terminalFocus";

type TerminalViewProps = {
  session: TerminalSession;
  searchShortcutActive?: boolean;
  active?: boolean;
  // inPane is whether this terminal is one of the panes of the Terminal section, not a session kept hidden behind
  // them. Such a terminal keeps its WebGL renderer while another section is open: building it again on every return
  // stopped the screen for a moment and drew the text with the DOM renderer's cells first, which wrap differently.
  inPane?: boolean;
  focusRequest?: number;
  headerActions?: ReactNode;
  onSplit?: (edge: PaneEdge) => void;
  // onRemoveFromSplit takes this session out of the split it is shown in.
  onRemoveFromSplit?: () => void;
  // splitRefusal says why nothing can go beside this session; undefined when something can.
  splitRefusal?: string | undefined;
  api?: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
  // onExit hands over how the program ended, as the stream reported it.
  onExit?: (exit: ExitReport) => void;
  // onProgramProgressChanged says the OSC 9;4 progress under the header changed, as the stream reported it.
  onProgramProgressChanged?: () => void;
  onReconnect?: () => Promise<boolean>;
  onStopReconnect?: () => Promise<boolean>;
  // onCloseSession closes this session; an ended session's card offers it.
  onCloseSession?: () => void;
  // onAnswerHostKey answers whether to trust the unknown host key the connection asks about (hostKeyQuestion).
  onAnswerHostKey?: (fingerprint: string, accept: boolean) => Promise<boolean>;
  // onAnswerPasswordSave answers whether to save the password typed for this connection (passwordSaveQuestion).
  onAnswerPasswordSave?: (save: boolean) => Promise<boolean>;
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
  inPane = true,
  focusRequest,
  headerActions,
  onSplit,
  onRemoveFromSplit,
  splitRefusal,
  api = terminalSessionsApi,
  onExit,
  onProgramProgressChanged,
  onReconnect,
  onStopReconnect,
  onCloseSession,
  onAnswerHostKey,
  onAnswerPasswordSave,
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
  const reducedMotion = useReducedMotion();
  const mobile = useMediaQuery(mobileViewportQuery);
  const touchTerminal = useTouchTerminal();
  const touchLinkUnderlines = useTouchLinkUnderlines();
  const everyLinkUnderlined = touchTerminal && touchLinkUnderlines;
  const resolvedFontSize = fontSize ?? (mobile ? mobileTerminalFontSize : desktopTerminalFontSize);
  // zoomedFontSize は、つまむ操作か Ctrl とホイールが、設定の大きさ（base）の上にこのターミナルへ与えた大きさで
  // ある。この画面だけのもので、保存しない。設定の大きさが変わったら、また設定に従う。
  const [zoomedFontSize, setZoomedFontSize] = useState<{ base: number; size: number } | null>(null);
  const shownFontSize = zoomedFontSize !== null && zoomedFontSize.base === resolvedFontSize ? zoomedFontSize.size : resolvedFontSize;
  const resolvedFontFamily = useTerminalFont(font ?? "", shownFontSize);
  const host = useRef<HTMLDivElement>(null);
  const statusLineHost = useRef<HTMLDivElement>(null);
  // shownStatusLineHeight は、プログラムが出す VT320 のステータス行（statusLine.ts）の高さである。出ていなければ
  // null。行を画面の下に描くのは設定がオンのときだけで、行への書き込みはどちらでも受ける。
  const [shownStatusLineHeight, setStatusLineHeight] = useState<number | null>(null);
  const statusLineHeight = statusLineEnabled ? shownStatusLineHeight : null;
  const region = useRef<HTMLElement>(null);
  const backgroundURL = useBackgroundImage(background ?? "");
  const backgroundConfigured = (background ?? "") !== "";
  const hasBackground = backgroundURL !== "";
  // assembled は組み立てたターミナル（terminalAssembly.ts）、terminal はその xterm.js である。組み立てる前と
  // 片付けたあとは null。
  const assembled = useRef<AssembledTerminal | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const [problem, setProblem] = useState("");
  const [link, setLink] = useState<StreamLink>({ phase: "connecting", attempt: 1 });
  const recovery = useSessionRecovery({
    onReconnect,
    onStopReconnect,
    onReconnected: () => assembled.current?.link.connect(),
  });
  // reconnectByEnter は、エンジンの行が Enter でつなぎ直すと言っているあいだの、つなぎ直す操作である。キーが
  // セッションへ届くあいだは null。
  const reconnectByEnter = reconnectsOnEnter(session) ? recovery.reconnect ?? null : null;
  const hostKeyAnswer = useHostKeyAnswer(onAnswerHostKey);
  const savedConnection = savedConnectionOf(session);
  const notices = useTerminalNotices({
    session,
    problem,
    link,
    reconnectFailed: recovery.reconnectFailed,
    answersHostKey: hostKeyAnswer.answer !== undefined,
    mobile,
  });
  // drawnLive は、リンクがつながっているときの形で画面が描かれているか（上に「接続中」の帯が無いか）である。
  // 下の layout effect が書く。
  const drawnLive = useRef(false);
  const commandCard = useCommandCard();
  // topCommand は、プロンプトが上に隠れているあいだ、その出力が画面の上を占めているコマンドである。
  const [topCommand, setTopCommand] = useState<CommandBlock | null>(null);
  const [osc52Enabled, setOsc52Enabled] = useState(initialOsc52Enabled);
  const [terminalNotice, setTerminalNotice] = useState("");
  const [pendingPaste, setPendingPaste] = useState<{
    sessionId: string;
    raw: string;
    // スマホの入力欄から来た文字である。送ったあとに Enter を続け、欄を空にする。
    fromComposer?: boolean;
  } | null>(null);
  const allowWindowsPaths = useLocalWindowsPaths(session.kind, runTarget);
  const [currentDirectory, setCurrentDirectory] = useState("");
  const {
    progress: programProgress,
    apply: applyProgramProgress,
    clear: clearProgramProgress,
  } = useProgramProgress(() => onProgramProgressChanged?.());

  // 組み立てたターミナルが、呼ばれた時点で読む値である（TerminalLiveValues）。描くたびに最新の値を入れる。
  // ターミナルが読む設定を足すときは、ここと型に足す。
  const liveValues: TerminalLiveValues = {
    session,
    t,
    active,
    inPane,
    touch: touchTerminal,
    everyLinkUnderlined,
    allowWindowsPaths,
    shownFontSize,
    settingsFontSize: resolvedFontSize,
    reducedMotion,
    clipboardSettings: { copyOnSelect, rightClickPaste },
    osc52Enabled,
    jisYenBackslash,
    commandMarks,
    commandScrollbarMarks,
    reconnectByEnter,
    onExit,
  };
  const live = useRef(liveValues);
  live.current = liveValues;

  const search = useTerminalSearch({
    shortcutActive: searchShortcutActive,
    region,
    // タッチの画面では、ターミナルへフォーカスを戻すとキーボードがまた上がる。
    returnFocus: () => {
      if (!live.current.touch) terminal.current?.focus();
    },
  });
  useAndroidBackStop({ open: search.open && active, close: search.close });

  const [quickCommandsOpen, setQuickCommandsOpen] = useState(false);
  const [quickCommandSelection, setQuickCommandSelection] = useState("");
  // パネルは、「…」のメニューかショートカットで開いたときに選んでいた文字から始まる。
  function openQuickCommands() {
    setQuickCommandSelection(terminal.current?.getSelection() ?? "");
    setQuickCommandsOpen(true);
  }
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

  const { modifiers, tap: tapModifier, takeModifiers } = useModifierKeys();
  const keyBarLayout = useKeyBarLayout();
  // 直接入力が続くのは、キーバーに切り替えのキーがあるあいだだけである。そのキーの無いプリセットでは、
  // 入力欄が隠れたまま戻せなくなる。
  const directInput = useDirectInput() && offersDirectInput(keyBarLayout);
  const composer = useTerminalComposer({
    session,
    link,
    reconnect: reconnectByEnter,
    bracketedPaste: () => terminal.current?.modes.bracketedPasteMode ?? false,
    sendInput: (text) => assembled.current?.send(text),
    sendPaste: (text) => assembled.current?.paste(text),
    confirmPaste: (text) => setPendingPaste({ sessionId: session.id, raw: text, fromComposer: true }),
    notice: setTerminalNotice,
  });
  const composerField = composer.fieldRef;
  const { setCursorAtSecretPrompt } = composer;

  useEffect(() => {
    setOsc52Enabled(initialOsc52Enabled);
    setCurrentDirectory("");
    clearProgramProgress();
    setPendingPaste(null);
  }, [clearProgramProgress, initialOsc52Enabled, session.id, session.state]);

  // 終わったと出していたセッションが、ここでの操作なしにつなぎ直された（ほかのブラウザが再接続した）ら、
  // 新しいシェルの出力を受け取り直す。
  const shownAsEnded = useRef(session.state === "exited");
  useEffect(() => {
    if (shownAsEnded.current && session.state !== "exited") assembled.current?.link.attachAfterExit();
    shownAsEnded.current = session.state === "exited";
  }, [session.state]);

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

  // ターミナルはセッションごとに1度だけ組み立てる（terminalAssembly.ts）。設定・関数・翻訳は live を通して
  // 読まれるか、下の effect が当て直すので、アドオンと WebSocket を作り直さない。
  useEffect(() => {
    const container = host.current;
    if (container === null) return;
    const built = assembleTerminal({
      container,
      live: () => live.current,
      start: {
        api,
        backgroundImage: backgroundConfigured,
        backgroundLoaded: hasBackground,
        fontFamily: resolvedFontFamily,
        scrollback: scrollbackLines,
        webgl: webglEnabled,
        ligatures,
      },
      wiring: {
        link: setLink,
        notice: setTerminalNotice,
        problem: setProblem,
        directory: setCurrentDirectory,
        progress: applyProgramProgress,
        statusLineHeight: setStatusLineHeight,
        topCommand: setTopCommand,
        pointCommand: commandCard.point,
        leaveCommand: commandCard.leave,
        closeCommandCard: commandCard.close,
        cursorAtSecretPrompt: setCursorAtSecretPrompt,
        reviewPaste: (text) => setPendingPaste({ sessionId: session.id, raw: text }),
        dropPasteReview: () => setPendingPaste(null),
        zoom: setZoomedFontSize,
        selectLink: selectLinkAt,
        openQuickCommands,
        takeModifiers,
        search,
        typingElsewhere: () => isTypingElsewhere(region.current),
        linkDrawnLive: () => drawnLive.current,
      },
    });
    assembled.current = built;
    terminal.current = built.view;
    return () => {
      built.dispose();
      assembled.current = null;
      terminal.current = null;
    };
    // oxlint-disable-next-line react-hooks/exhaustive-deps
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
        const link = linkAtPoint(view, point, live.current.allowWindowsPaths);
        if (link !== null) {
          selectLinkAt(link, point);
          return true;
        }
        // A tap on the line being typed also moves the cursor there; it still
        // raises the keyboard to type at the new place.
        const cell = cellAtPoint(view, point);
        const keys = cell === null ? null : assembled.current?.promptCursor.arrowsTo(cell) ?? null;
        if (keys !== null) assembled.current?.send(keys);
        return false;
      },
      // A phone types into its text box, where Japanese is composed before it is sent; the terminal
      // takes the keys typed with the key bar's Ctrl or Alt (holdModifier below).
      typingTarget: () => composerField.current ?? view,
    });
    assembled.current?.fit();
    return () => {
      releaseImeKeys();
      detachOverlay();
    };
  }, [touchTerminal, selectLinkAt, composerField, session.id, api, backgroundConfigured, webglEnabled]);

  // Showing a session (opening one, picking one, or the one left after closing another) focuses it. A touch
  // terminal is left alone: focusing it raises the software keyboard, which comes up only for a tap on the terminal.
  useEffect(() => {
    if (focusRequest !== undefined && live.current.active && !live.current.touch && !region.current?.contains(document.activeElement)) terminal.current?.focus();
  }, [focusRequest]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  // A touch terminal underlines every link, unless the preset turns it off; Windows paths become links once the
  // engine is known to run on Windows.
  useEffect(() => {
    assembled.current?.linkUnderlines.refresh();
  }, [everyLinkUnderlined, allowWindowsPaths]);

  // The command marks follow their settings and the theme's colours.
  const { pointed: pointedCommand, close: closeCommandCard } = commandCard;
  useEffect(() => {
    assembled.current?.commandDisplay.refresh();
    if (!commandMarks) closeCommandCard();
  }, [commandMarks, commandScrollbarMarks, resolved, palette, closeCommandCard]);

  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.fontFamily = resolvedFontFamily;
    assembled.current?.fit();
  }, [resolvedFontFamily]);

  // A session that leaves the panes (kept behind them) gives its WebGL renderer back, even while the section is hidden.
  useEffect(() => {
    assembled.current?.fit();
  }, [inPane]);

  // A stream is attached as the link turns live, while the banner that says it is connecting still
  // takes a row or two above the screen. A size measured then would be followed by the right one
  // once the banner goes, and a shell or a full-screen program would redraw twice. A layout effect
  // runs after React has drawn the screen without the banner.
  useLayoutEffect(() => {
    drawnLive.current = link.phase === "live";
    if (drawnLive.current) assembled.current?.fit();
  }, [link.phase]);

  useEffect(() => {
    assembled.current?.ligatureDrawing?.set(ligatures);
  }, [ligatures]);

  useEffect(() => {
    if (statusLineHeight !== null && statusLineHost.current !== null) assembled.current?.statusLine.open(statusLineHost.current);
  }, [statusLineHeight]);

  useEffect(() => {
    if (terminal.current === null) return;
    const changed = applyTerminalRuntimeOptions(terminal.current.options, {
      cursorBlink: session.state !== "exited" && cursorAnimationEnabled(reducedMotion),
      fontSize: shownFontSize,
      scrollback: scrollbackLines,
    });
    if (changed.refit) assembled.current?.fit();
  }, [shownFontSize, reducedMotion, scrollbackLines, session.state]);

  const displayTitle = terminalDisplayTitle(session);
  const subtitle = terminalSubtitle(session, t);
  // Ctrl and Alt apply to the next key typed into the terminal, so one held while typing in the text box
  // moves the typing there, keeping the keyboard up. A closed keyboard stays closed: the key bar has already
  // let go of the focus then (keepKeyboardAsIs).
  function holdModifier(name: ModifierName) {
    if (modifiers[name] === "off" && document.activeElement === composerField.current) terminal.current?.focus();
    tapModifier(name);
  }

  // switchDirectInput moves where a phone types, keeping an open keyboard open: the focus goes from the text
  // box to the terminal, or back to the text box, which comes back first (flushSync) so it can take the focus.
  function switchDirectInput() {
    if (!directInput) {
      if (document.activeElement === composerField.current) terminal.current?.focus();
      setDirectInput(true);
      composer.forgetSecret();
      return;
    }
    const typingInTerminal = terminal.current?.textarea !== undefined && document.activeElement === terminal.current.textarea;
    flushSync(() => setDirectInput(false));
    if (typingInTerminal) composerField.current?.focus();
  }

  function resumeProgram(command: string) {
    assembled.current?.send(`${command}\r`);
    if (!touchTerminal) terminal.current?.focus();
  }

  const statusBanners = (
    <TerminalStatusBanners
      session={session}
      problem={problem}
      link={link}
      onLinkNow={() => assembled.current?.link.connect()}
      onLinkStop={() => assembled.current?.link.stopRetrying()}
      recovery={recovery}
      hostKeyAnswer={hostKeyAnswer}
      onResumeProgram={resumeProgram}
      hiddenRows={notices.stripHiddenRows}
    />
  );

  return (
    <section ref={region} aria-label={t("terminal.screenLabel", { title: displayTitle })} className="relative flex min-h-0 flex-1 flex-col">
      <TerminalHeader session={session} link={link} mobile={mobile} vpnProfile={vpnProfile} runTarget={runTarget} onOpenNavigation={onOpenNavigation}>
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
        {mobile && notices.severity !== null ? (
          <TerminalNoticesButton severity={notices.severity} expanded={notices.mobileOpen} onToggle={notices.toggleMobile} buttonRef={notices.buttonRef} />
        ) : null}
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
              onPreviousPrompt: () => assembled.current?.prompts.jump(-1),
              onNextPrompt: () => assembled.current?.prompts.jump(1),
            } : {})}
            onSplit={onSplit}
            onRemoveFromSplit={onRemoveFromSplit}
            splitRefusal={splitRefusal}
            triggerRef={overflowTrigger}
            osc52Enabled={osc52Enabled}
            onQuickCommands={openQuickCommands}
            onPortForwarding={session.kind === "ssh" ? () => setPortForwardsOpen(true) : undefined}
            onOpenRemoteDirectory={runTarget === undefined && savedConnection !== undefined && currentDirectory !== "" && onOpenRemotePath !== undefined
              ? () => onOpenRemotePath(savedConnection, currentDirectory, "browse")
              : undefined}
            onCopyContext={() => assembled.current?.copy.copyContext()}
            onCopyWithColors={() => assembled.current?.copy.copyWithColors()}
            onSaveScrollback={() => assembled.current?.copy.saveScrollback()}
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
      </TerminalHeader>
      {mobile ? null : statusBanners}

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
        {notices.stripShown ? (
          // Over the top of the terminal, not above it: the terminal keeps its rows, and its program does not draw again.
          <div ref={notices.stripRef} tabIndex={-1} className="absolute inset-x-0 top-0 z-20 bg-canvas shadow-float outline-none">
            {statusBanners}
          </div>
        ) : null}
        {notices.card === null || !notices.cardShown ? null : (
          // Low in the terminal, where the person's eyes are after the last line; the strip above keeps saying it.
          <div ref={notices.cardFrameRef} className="pointer-events-none absolute inset-x-3 bottom-3 z-20 flex justify-center mobile-layout:inset-x-2 mobile-layout:bottom-2">
            <TerminalNoticeCard
              // A new notice is a new card, which takes the focus again.
              key={notices.card.key}
              notice={notices.card}
              session={session}
              title={displayTitle}
              recovery={recovery}
              hostKeyAnswer={hostKeyAnswer}
              notificationPermission={notices.notificationPermission}
              focus={{
                // Only the terminal the person works in, and not from under a dialog or menu. A phone shows it when the
                // person asks for the notices, so it always takes the focus there. The hint about the previous shell's
                // program leaves the focus alone: the new shell is there to type into.
                take: () => mobile || (notices.card?.kind !== "resume-program" && active && !isDismissibleLayerOpen()),
                // A touch screen would raise its keyboard again for the terminal.
                giveBack: () => {
                  if (!live.current.touch) terminal.current?.focus();
                },
              }}
              onLinkNow={() => assembled.current?.link.connect()}
              onCloseSession={onCloseSession}
              onAnswerPasswordSave={onAnswerPasswordSave}
              onDismiss={notices.dismissCard}
              onResumeProgram={resumeProgram}
            />
          </div>
        )}
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
          onCopy={() => assembled.current?.copy.copyCommandOutput(pointedCommand.block)}
          onSave={() => assembled.current?.copy.saveCommandOutput(pointedCommand.block)}
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
            assembled.current?.paste(raw);
            if (pendingPaste.fromComposer === true) {
              assembled.current?.send("\r");
              composer.setText("");
            }
          }}
        />
      )}
      <KeyBar
        modifiers={modifiers}
        onTap={holdModifier}
        layout={keyBarLayout}
        directInput={directInput}
        onKey={(key) => assembled.current?.sendKey(key)}
        onDirectInput={switchDirectInput}
      />
      {directInput ? null : <TerminalComposer
        value={composer.text}
        secret={composer.secret}
        sendable={composer.sendable}
        placeholder={composer.placeholder}
        enterSends={composerEnterSends}
        fieldRef={composer.fieldRef}
        onChange={composer.setText}
        onSecretChange={composer.setSecret}
        onSubmit={composer.submit}
      />}
    </section>
  );
}

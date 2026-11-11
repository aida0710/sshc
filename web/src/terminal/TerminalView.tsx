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
  // inPane は、このターミナルが Terminal の画面に並ぶペインの1つかである。ペインの後ろに隠して持っている
  // セッションでは false。ペインは、ほかの画面が開いているあいだも WebGL のレンダラーを持ち続ける。戻るたびに
  // 作り直すと、画面が一瞬止まり、折り返しの違う DOM のレンダラーのマスで先に文字が描かれた。
  inPane?: boolean;
  focusRequest?: number;
  headerActions?: ReactNode;
  onSplit?: (edge: PaneEdge) => void;
  // onRemoveFromSplit は、このセッションを、出ている分割から外す。
  onRemoveFromSplit?: () => void;
  // splitRefusal は、このセッションの横に何も置けない理由である。置けるときは undefined。
  splitRefusal?: string | undefined;
  api?: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath" | "terminalSessionDirectory">;
  // onExit は、ストリームが伝えた、プログラムの終わり方を渡す。
  onExit?: (exit: ExitReport) => void;
  // onProgramProgressChanged は、見出しの下の OSC 9;4 の進み具合が変わったことを、ストリームが伝えたときに呼ぶ。
  onProgramProgressChanged?: () => void;
  onReconnect?: () => Promise<boolean>;
  onStopReconnect?: () => Promise<boolean>;
  // onCloseSession は、このセッションを閉じる。終わったセッションのカードが出す。
  onCloseSession?: () => void;
  // onAnswerHostKey は、接続が尋ねている知らないホスト鍵を信頼するかに答える（hostKeyQuestion）。
  onAnswerHostKey?: (fingerprint: string, accept: boolean) => Promise<boolean>;
  // onAnswerPasswordSave は、この接続に打ち込んだパスワードを保存するかに答える（passwordSaveQuestion）。
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
  // onOpenNavigation は、スマホの上のバーからサイドバーを開く。スマホでは、このバーがアプリの見出しの代わりになる。
  onOpenNavigation?: (() => void) | undefined;
  osc52Enabled?: boolean;
  // vpnProfile は、この接続が通るVPN経路の名前である。空なら通らない。
  vpnProfile?: string;
  // runTarget は、このセッションが動いているエンジンの SSH の別名である。このマシンで動いていれば undefined。
  // クイックコマンドとポート転送は、そのエンジンへ送る。パスを SFTP で開く操作は、このマシンから同じ行き先に
  // 届くときだけ出す。
  runTarget?: string;
  scrollbackLines?: number;
  onOsc52Change?: (enabled: boolean) => void | Promise<void>;
  onForwardsChanged?: () => void | Promise<void>;
  jisYenBackslash?: boolean;
  // composerEnterSends: スマホの入力欄で Enter が送る。そうでなければ Enter は改行する。
  composerEnterSends?: boolean;
  // ligatures は、-> や != のような並びを、フォントの合字で描く。
  ligatures?: boolean;
  // statusLine は、プログラムが出す VT320 のステータス行（statusLine.ts）を出す。オフのときは、そこに書かれた
  // ものも画面に出さない。
  statusLine?: boolean;
  // シェル統合（OSC 133）があるときの、各コマンドの左の印（commandGutter.ts）、出力をスクロールしているあいだ
  // 上に出しておくコマンド、スクロールバーの印。
  commandMarks?: boolean;
  stickyCommand?: boolean;
  commandScrollbarMarks?: boolean;
};

// ステータス行は、画面からちょうど1行を取る。その行の上下の余白（index.css）は、文字の無い、画面の最後の行の
// 下の余白に重なる。
const statusLinePadding = "0.5rem";

// リンクのポップオーバーは、押したリンクを隠さないよう、ポインターの少し右下に開く。
const linkPopoverOffsetPx = 8;

// 設定が大きさを決めていないとき、スマホ（mobileViewportQuery。ツールバーや KeyBar と同じ条件）では、小さい
// 画面でも読めるよう、大きめにする。
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

  // ファイルのパスは、メニューを挟まずにプレビューを開く。そのファイルにできるほかの操作は、プレビューの「…」に
  // ある。URL と、ここでは読めないファイルのパスは、メニューを開く。
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
    // セッション・api・背景の有無・WebGL 以外の値が変わっても、組み立て直さない。
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id, api, backgroundConfigured, webglEnabled]);

  // ターミナルを組み立てる effect のあとに宣言し、同じ値をキーにするので、新しいターミナルごとに付け直す。
  // タッチとマウスの切り替えで変わるのはここだけで、接続は変わらない。
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
        // 打っている行を押すと、カーソルもそこへ動く。その場所で打てるよう、キーボードも上げる。
        const cell = cellAtPoint(view, point);
        const keys = cell === null ? null : assembled.current?.promptCursor.arrowsTo(cell) ?? null;
        if (keys !== null) assembled.current?.send(keys);
        return false;
      },
      // スマホは入力欄に打つ。日本語は、送る前にそこで変換する。キーバーの Ctrl や Alt を付けて打つキーは、
      // ターミナルが受け取る（下の holdModifier）。
      typingTarget: () => composerField.current ?? view,
    });
    assembled.current?.fit();
    return () => {
      releaseImeKeys();
      detachOverlay();
    };
  }, [touchTerminal, selectLinkAt, composerField, session.id, api, backgroundConfigured, webglEnabled]);

  // セッションを出す（開く、選ぶ、ほかを閉じて残った）と、そのセッションにフォーカスする。タッチのターミナルは
  // そのままにする。フォーカスするとソフトウェアキーボードが上がる。キーボードは、ターミナルを押したときだけ
  // 上がる。
  useEffect(() => {
    if (focusRequest !== undefined && live.current.active && !live.current.touch && !region.current?.contains(document.activeElement)) terminal.current?.focus();
  }, [focusRequest]);

  useEffect(() => {
    if (terminal.current === null || host.current === null) return;
    terminal.current.options.theme = terminalTheme(host.current, hasBackground);
  }, [resolved, palette, hasBackground]);

  // タッチのターミナルは、プリセットがオフにしていなければ、すべてのリンクに下線を引く。Windows のパスは、
  // エンジンが Windows で動いていると分かったあとにリンクになる。
  useEffect(() => {
    assembled.current?.linkUnderlines.refresh();
  }, [everyLinkUnderlined, allowWindowsPaths]);

  // コマンドの印は、その設定とテーマの色に従う。
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

  // ペインから外れた（後ろに持っている）セッションは、画面が隠れているあいだでも、WebGL のレンダラーを返す。
  useEffect(() => {
    assembled.current?.fit();
  }, [inPane]);

  // ストリームは、リンクがつながると同時に付く。そのとき、「接続中」の帯がまだ画面の上の1〜2行を取っている。
  // そこで測った大きさのあとに、帯が消えた正しい大きさが続くと、シェルや全画面のプログラムが2度描き直す。
  // layout effect は、React が帯の無い画面を描いたあとに走る。
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
  // Ctrl と Alt は、ターミナルに打つ次のキーに効く。入力欄に打っている最中に押すと、キーボードを上げたまま、
  // 打つ先をターミナルへ移す。閉じているキーボードは閉じたままにする。そのとき、キーバーはすでにフォーカスを
  // 手放している（keepKeyboardAsIs）。
  function holdModifier(name: ModifierName) {
    if (modifiers[name] === "off" && document.activeElement === composerField.current) terminal.current?.focus();
    tapModifier(name);
  }

  // switchDirectInput は、開いているキーボードを開いたまま、スマホの打つ先を切り替える。フォーカスは入力欄から
  // ターミナルへ移るか、入力欄へ戻る。戻るときは、フォーカスを受け取れるよう、先に入力欄を出す（flushSync）。
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
            className="flex size-7 shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink focus:bg-hover focus:outline-none"
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
          className={`flex shrink-0 items-center justify-center text-ink-faint hover:bg-hover hover:text-ink focus:bg-hover focus:outline-none rounded-md ${mobile ? "size-11" : "size-7"}`}
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

      <div className={`relative min-h-0 flex-1 overflow-clip bg-term-bg ${mobile ? "mx-2 rounded-card border border-line" : ""}`}>
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
            // 画面は、ステータス行に1行を譲る。大きさの見張りが合わせ直す。
            ...(statusLineHeight === null ? {} : { bottom: `${statusLineHeight}px` }),
          }}
          // 画面に置いた2本の指は、ページではなく、ターミナルの文字をつまむ（attachFontZoom）。
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
          // ターミナルの上に足すのではなく、上の端に重ねる。ターミナルは行数を保ち、プログラムは描き直さない。
          <div ref={notices.stripRef} tabIndex={-1} className="absolute inset-x-0 top-0 z-20 bg-canvas shadow-float outline-none">
            {statusBanners}
          </div>
        ) : null}
        {notices.card === null || !notices.cardShown ? null : (
          // ターミナルの下のほうに出す。最後の行を読んだあと、利用者の目はそこにある。同じことは、上の帯も伝え続ける。
          <div ref={notices.cardFrameRef} className="pointer-events-none absolute inset-x-3 bottom-3 z-20 flex justify-center mobile-layout:inset-x-2 mobile-layout:bottom-2">
            <TerminalNoticeCard
              // 新しい知らせは新しいカードで、フォーカスを取り直す。
              key={notices.card.key}
              notice={notices.card}
              session={session}
              title={displayTitle}
              recovery={recovery}
              hostKeyAnswer={hostKeyAnswer}
              notificationPermission={notices.notificationPermission}
              focus={{
                // 利用者が作業しているターミナルだけがフォーカスを取り、ダイアログやメニューの下からは取らない。スマホでは、
                // 利用者が知らせを開いたときに出すので、いつも取る。前のシェルのプログラムについての案内は、フォーカスに
                // 触れない。新しいシェルが、打ち込めるようにそこにある。
                take: () => mobile || (notices.card?.kind !== "resume-program" && active && !isDismissibleLayerOpen()),
                // タッチの画面では、ターミナルへフォーカスを戻すとキーボードがまた上がる。
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

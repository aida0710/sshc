import type { Dispatch, SetStateAction } from "react";
import type { SearchAddon } from "@xterm/addon-search";
import type { Terminal } from "@xterm/xterm";
import type { TerminalSession, TerminalSessionsApi } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { keyBarInput, withModifiers, type KeyBarKey, type KeyModifiers } from "../keybar/keySequences";
import { clipboard } from "../ui/clipboard";
import { cursorAnimationEnabled } from "../ui/reducedMotion";
import { attachFontZoom } from "./attachFontZoom";
import { attachTerminalSequences } from "./attachTerminalSequences";
import { attachTerminalShortcuts } from "./attachTerminalShortcuts";
import { attachTerminalClipboard, prepareTerminalPaste, type TerminalClipboardSettings } from "./clipboard";
import type { CommandBlock } from "./commandBlocks";
import { attachCommandDisplay, type CommandDisplay } from "./commandDisplay";
import { createTerminalView } from "./createTerminalView";
import { resizeRewritingCursorLine } from "./cursorLineRewrite";
import { textBeforeCursor } from "./cursorLineText";
import { steppedFontSize } from "./fontSize";
import { encodeIntlYen } from "./intlYenKey";
import type { LigatureDrawing } from "./ligatures";
import { attachLinkProvider } from "./linkProvider";
import { attachLinkUnderlines, type LinkUnderlines } from "./linkUnderlines";
import { cellHeight, observeTerminalSize, syncTerminalInputPosition } from "./metrics";
import type { TerminalProgress } from "./osc9Progress";
import { createTerminalOutput } from "./output";
import { inspectTerminalPaste } from "./pasteGuard";
import { attachPromptCursorClicks, type PromptCursor } from "./promptCursor";
import { attachPromptJumpKeys, type PromptJump } from "./promptJump";
import { createScrollbackOffset } from "./scrollbackOffset";
import { asksForSecret, secretPromptRowsRead } from "./secretPrompt";
import { selectionHeldIn } from "./selectionOverlay";
import { createStatusLineRouter } from "./statusLine";
import { createStatusLineTerminal, type StatusLineTerminal } from "./statusLineTerminal";
import type { ExitReport } from "./stream";
import { createStreamLink, type StreamLink, type StreamLinkController } from "./streamLink";
import { cellAtPoint, type ScreenPoint } from "./tappedLink";
import { registerTerminalActions } from "./terminalActions";
import { terminalCopyActions, type TerminalCopyActions } from "./terminalCopyActions";
import { isInNoticeCard } from "./terminalFocus";
import { openTerminalURL, type TerminalLinkSelection } from "./TerminalLinkPopover";
import { terminalDisplayTitle } from "./terminalPresentation";
import { terminalTheme } from "./theme";
import { attachTouchScroll } from "./touchScroll";
import { terminalTouchScrollTarget } from "./touchScrollTarget";
import { isTerminalShown, watchTerminalShown } from "./terminalVisibility";

// TerminalLiveValues は、組み立てたターミナルが、呼ばれた時点で読む値である。ターミナルはセッションごとに
// 1度しか組み立てないので、あとから変わる値はすべてここを通して読む。画面は、描くたびに最新の値を入れる。
export type TerminalLiveValues = {
  session: TerminalSession;
  t: Translate;
  // active は、利用者が今このターミナルで作業しているかである。
  active: boolean;
  // inPane は、Terminal の画面に並ぶペインの1つかである（TerminalView の inPane）。
  inPane: boolean;
  // touch は、指で操作するターミナルかである。
  touch: boolean;
  everyLinkUnderlined: boolean;
  allowWindowsPaths: boolean;
  // shownFontSize は今の文字の大きさ、settingsFontSize は設定が決めた大きさである。
  shownFontSize: number;
  settingsFontSize: number;
  reducedMotion: boolean;
  clipboardSettings: TerminalClipboardSettings;
  osc52Enabled: boolean;
  jisYenBackslash: boolean;
  commandMarks: boolean;
  commandScrollbarMarks: boolean;
  // reconnectByEnter は、エンジンの行が Enter でつなぎ直すと言っているあいだの、つなぎ直す操作である。
  // キーがセッションへ届くあいだは null。
  reconnectByEnter: (() => void) | null;
  onExit: ((exit: ExitReport) => void) | undefined;
  // onProgramStateChanged は、プログラムが何をしているかを知らせた（OSC 7501）ときに呼ぶ。
  onProgramStateChanged: (() => void) | undefined;
};

// TerminalStartOptions は、組み立てるときに1度だけ読む値である。変わったら組み立て直すか、画面が
// 組み立てたあとのターミナルへ当て直す。
export type TerminalStartOptions = {
  api: Pick<TerminalSessionsApi, "terminalStreamTicket" | "streamPath">;
  // backgroundImage は背景の画像が設定されているか、backgroundLoaded はその画像を読めたかである。
  backgroundImage: boolean;
  backgroundLoaded: boolean;
  fontFamily: string;
  scrollback: number;
  webgl: boolean;
  ligatures: boolean;
};

// TerminalWiring は、ターミナルで起きたことを画面へ伝える先と、画面の持ち物でターミナルが使うものである。
// どれも画面が生きているあいだ同じ関数なので、組み立てるときに1度受け取る。
export type TerminalWiring = {
  link: (link: StreamLink) => void;
  // notice は少しのあいだ出す知らせ、problem は断られた理由を伝える。
  notice: (message: string) => void;
  problem: (message: string) => void;
  directory: (directory: string) => void;
  progress: Dispatch<SetStateAction<TerminalProgress>>;
  // statusLineHeight は、プログラムが出すステータス行の高さを伝える。出ていなければ null。
  statusLineHeight: (height: number | null) => void;
  topCommand: (block: CommandBlock | null) => void;
  pointCommand: (block: CommandBlock, mark: DOMRect) => void;
  leaveCommand: () => void;
  closeCommandCard: () => void;
  cursorAtSecretPrompt: (asking: boolean) => void;
  // reviewPaste は、確かめてから送る貼り付けを画面に渡す。dropPasteReview は、確かめている貼り付けを取り下げる。
  reviewPaste: (text: string) => void;
  dropPasteReview: () => void;
  // zoom は、このターミナルだけの文字の大きさ（size）と、そのときの設定の大きさ（base）を伝える。null は
  // 設定の大きさに戻す。
  zoom: (zoomed: { base: number; size: number } | null) => void;
  selectLink: (link: TerminalLinkSelection["link"], point: ScreenPoint) => void;
  openQuickCommands: () => void;
  takeModifiers: () => KeyModifiers;
  search: {
    bind: (view: Terminal, search: SearchAddon, container: HTMLElement) => () => void;
    hasFocus: () => boolean;
    isSelectingMatch: () => boolean;
    show: () => void;
  };
  // typingElsewhere は、利用者がこのターミナルの画面のほかの欄に打っているかを返す。
  typingElsewhere: () => boolean;
  // linkDrawnLive は、リンクがつながっているときの形で画面が描かれているか（上に「接続中」の帯が無いか）を返す。
  linkDrawnLive: () => boolean;
};

// AssembledTerminal は、組み立てた1つのセッションのターミナルである。
export type AssembledTerminal = {
  view: Terminal;
  // fit は、ターミナルを入れ物の大きさに合わせ直し、その大きさをエンジンへ伝える。
  fit: () => void;
  // send は、打ったものとして送る。sendKey は、キーバーのキーを送る。paste は、貼り付けとして送る。
  send: (text: string) => void;
  sendKey: (key: KeyBarKey) => void;
  paste: (text: string) => void;
  copy: TerminalCopyActions;
  prompts: PromptJump;
  promptCursor: PromptCursor;
  commandDisplay: CommandDisplay;
  linkUnderlines: LinkUnderlines;
  ligatureDrawing: LigatureDrawing | null;
  statusLine: StatusLineTerminal;
  link: StreamLinkController;
  dispose: () => void;
};

// assembleTerminal は、container の中に1つのセッションのターミナルを組み立てる。xterm.js とそのアドオン、
// エンジンとのストリーム、入力、コピーと貼り付け、リンク、コマンドの印をつなぎ、ストリームをつなぎ始める。
export function assembleTerminal({ container, live, start, wiring }: {
  container: HTMLElement;
  live: () => TerminalLiveValues;
  start: TerminalStartOptions;
  wiring: TerminalWiring;
}): AssembledTerminal {
  const sessionId = live().session.id;
  const translate: Translate = (key, values) => live().t(key, values);
  // fitLater は、あとで作る fitAndSync を、先に作る部品から呼ぶための入口である。
  let fitLater: () => void = () => {};

  const parts = createTerminalView(container, {
    backgroundImage: start.backgroundImage,
    cursorBlink: live().session.state !== "exited" && cursorAnimationEnabled(live().reducedMotion),
    fontFamily: start.fontFamily,
    fontSize: live().shownFontSize,
    theme: terminalTheme(container, start.backgroundLoaded),
    scrollback: start.scrollback,
    webgl: start.webgl,
    webglSettled: () => fitLater(),
    ligatures: start.ligatures,
    selectLink: wiring.selectLink,
  });
  const { view, fit, searchAddon, serializeAddon, renderer } = parts;
  const measured = () => container.clientWidth !== 0 && container.clientHeight !== 0;
  // ペインは、ほかの画面に隠れているあいだも WebGL のレンダラーを持ち続ける（TerminalView の inPane）。
  const wantsRenderer = () => measured() || live().inPane;
  renderer.shown(wantsRenderer());

  const unbindSearch = wiring.search.bind(view, searchAddon, container);
  const copy = terminalCopyActions({
    view,
    serializeAddon,
    title: () => terminalDisplayTitle(live().session),
    t: translate,
    notice: wiring.notice,
    problem: wiring.problem,
  });

  const statusLine = createStatusLineTerminal(view);
  // replaySizeHeld は、履歴を、書かれたときの大きさで流し直しているあいだ真である（output.ts）。そのあいだ
  // ターミナルは入れ物に合わせず、その大きさをエンジンへも伝えない。
  let replaySizeHeld = false;
  const output = createTerminalOutput(createStatusLineRouter({
    screen: view,
    statusLine: () => statusLine,
    shownChanged: (shown) => {
      if (shown) statusLine.clear();
      wiring.statusLineHeight(shown ? cellHeight(view, container) : null);
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
    t: translate,
    replaying: () => output.isReplaying(),
    clipboardWritesAllowed: () => live().osc52Enabled,
    reply: (sequence) => send(sequence),
    notice: wiring.notice,
    problem: wiring.problem,
    setDirectory: wiring.directory,
    setProgress: wiring.progress,
    programStatusReported: () => live().onProgramStateChanged?.(),
  });
  const { kittyClipboard, prompt: promptCursor, prompts } = sequences;
  const commandDisplay = attachCommandDisplay(view, sequences.commandBlocks, {
    container,
    marks: () => live().commandMarks,
    scrollbarMarks: () => live().commandScrollbarMarks,
    point: wiring.pointCommand,
    leave: wiring.leaveCommand,
    topChanged: wiring.topCommand,
  });
  const detachPromptJumpKeys = attachPromptJumpKeys(container, prompts);
  const detachPromptClicks = attachPromptCursorClicks({
    container,
    prompt: promptCursor,
    cellAt: (point) => cellAtPoint(view, point),
    hasSelection: () => view.hasSelection(),
    enabled: () => !live().touch,
    send: (keys) => send(keys),
  });
  const linkUnderlines = attachLinkUnderlines(view, {
    everyLink: () => live().everyLinkUnderlined,
    allowWindowsPaths: () => live().allowWindowsPaths,
  });
  const terminalLinks = attachLinkProvider(view, {
    open: openTerminalURL,
    select: wiring.selectLink,
    point: linkUnderlines.point,
    allowWindowsPaths: () => live().allowWindowsPaths,
  });

  // sentSize は、今のストリームへ最後に伝えた大きさである。
  let sentSize = "";
  const syncSize = () => {
    // 隠れているセッションを出し直すときは、入れ物を測れるまで PTY の大きさを保つ。xterm の最初の 80x24 を
    // 送らない。
    if (!measured()) return;
    // WebGL のレンダラーを読み込んでいるあいだのマスは DOM のレンダラーのもので、WebGL が描く整数の
    // ピクセルより広い。プログラムは少ない桁数を受け取り、そのあと正しい桁数を受け取ることになる。
    // レンダラーが落ち着いたら合わせ直す。
    if (renderer.attaching()) return;
    if (!wiring.linkDrawnLive()) return;
    const stream = link.currentStream();
    const size = `${view.cols}x${view.rows}`;
    if (stream === null || view.cols === 0 || view.rows === 0 || size === sentSize) return;
    sentSize = size;
    stream.resize(view.cols, view.rows);
  };

  const link = createStreamLink({
    sessionId,
    api: start.api,
    onLink: wiring.link,
    onAttached: () => {
      sentSize = "";
      output.abandonReplay();
      // 新しいストリームは、エンジンから見ると「見えている」で始まる。本当のところをすぐ伝える。
      link.currentStream()?.reportShown(isTerminalShown(container));
      const current = live();
      if (current.active && !current.touch && current.session.state !== "exited" && !wiring.search.hasFocus() && !wiring.typingElsewhere() && !isInNoticeCard()) {
        view.focus();
      }
      fitAndSync();
    },
    onReplay: (replay, resumed) => {
      output.beginReplay(replay, { sized: !resumed });
      if (!replay.truncated) return;
      // 出力を失ったのは、切れていたリンクだけである。出し直した画面に無いのは、セッションのいちばん古い
      // 出力だけである。
      wiring.notice(translate(resumed ? "terminal.replayTruncated" : "terminal.replayTruncatedOldest"));
    },
    onSkipped: (screen) => {
      output.skip(screen);
      wiring.notice(translate("terminal.outputSkipped"));
    },
    // 出力だけでは、確かめている貼り付けを取り下げない。全画面のプログラムはいつも描き直すし、確認を
    // 開いたときのフォーカスの変化でも描き直す。
    onOutput: output.write,
    // 貼り付けは、もう無いシェルに向けて確かめたものである。
    onShellReplaced: wiring.dropPasteReview,
    onExit: (exit) => {
      view.options.cursorBlink = false;
      wiring.dropPasteReview();
      live().onExit?.(exit);
    },
    onClose: () => {
      output.abandonReplay();
      wiring.dropPasteReview();
    },
  });

  const fitAndSync = () => {
    renderer.shown(wantsRenderer());
    if (!measured() || replaySizeHeld) return;
    try {
      // エンジンがまだつないでいるあいだ、カーソルのある行はエンジン自身の問い（ホスト鍵、パスワード）で、
      // 新しい幅で描き直すものがいない（resizeRewritingCursorLine）。
      const asking = live().session.state === "connecting" || live().session.state === "reconnecting";
      if (asking) resizeRewritingCursorLine(view, () => fit.fit());
      else fit.fit();
      if (live().touch) syncTerminalInputPosition(view);
    } catch {
      return;
    }
    syncSize();
    view.refresh(0, Math.max(0, view.rows - 1));
  };
  // キーボードや向きが変わったら、文字を選んでいる最中でも PTY の大きさを変える。選択の重ね絵は、形が
  // 変わると取っ手を放す。最初の合わせは、残りをつないだあとで observeTerminalSize が行う。
  fitLater = fitAndSync;

  // 指はスクロールバックを、ブラウザのページのようにピクセル単位で動かす。xterm.js だけでは1行ずつ動く。
  const scrollbackOffset = createScrollbackOffset(container, view);
  const detachTouchScroll = attachTouchScroll(container, terminalTouchScrollTarget(view, scrollbackOffset), () => cellHeight(view, container), {
    selectionHeld: () => selectionHeldIn(container),
    reducedMotion: () => live().reducedMotion,
  });
  const zoomTo = (size: number) => wiring.zoom({ base: live().settingsFontSize, size });
  const detachFontZoom = attachFontZoom(container, {
    fontSize: () => live().shownFontSize,
    setFontSize: zoomTo,
  });
  // つまむ操作と同じく、文字の大きさのキーはこのターミナルだけを変え、保存しない。
  const shortcutHandlers = {
    increaseFontSize: () => zoomTo(steppedFontSize(live().shownFontSize, 1)),
    decreaseFontSize: () => zoomTo(steppedFontSize(live().shownFontSize, -1)),
    resetFontSize: () => wiring.zoom(null),
    quickCommands: wiring.openQuickCommands,
  };
  const detachShortcuts = attachTerminalShortcuts(container, shortcutHandlers);

  // disposed は、片付けたあと真である。片付けたターミナルへは何も送らない。
  let disposed = false;
  const send = (text: string) => {
    if (disposed) return;
    // Enter でつなぎ直すあいだ、ほかのキーはどこへも行かない。つなぎ直したシェルに打たれると、コマンドとして
    // 走ってしまう。
    const reconnect = live().reconnectByEnter;
    if (reconnect !== null) {
      if (text === "\r") reconnect();
      return;
    }
    link.currentStream()?.send(text);
  };
  const paste = (text: string) => {
    if (disposed) return;
    link.currentStream()?.send(prepareTerminalPaste(text, view.modes.bracketedPasteMode));
  };
  view.onData((data) => {
    // xterm は、流し直した履歴の中の問い合わせ（DA1、カーソルの位置、kitty の画像の確認）に、新しい問い合わせの
    // ように答える。聞いたプログラムはとうに答えを受け取っていて、この答えは打った文字としてシェルに入る。
    if (output.isReplaying()) return;
    send(withModifiers(data, wiring.takeModifiers()));
  });
  // 届いたものが画面に出たあと、多くても1フレームに1度呼ばれる。流し直した画面もここを通る。
  const secretPromptWatch = view.onWriteParsed(() => wiring.cursorAtSecretPrompt(asksForSecret(textBeforeCursor(view, secretPromptRowsRead))));
  // SGR（1006）なしでマウスの報告を有効にしたプログラムは、既定の形式で報告を受け取る。xterm.js はそれを
  // onData ではなくここへ渡す。
  view.onBinary((data) => link.currentStream()?.sendBinary(data));

  link.connect();

  const terminalClipboard = attachTerminalClipboard({
    container,
    terminal: view,
    paste: (text) => {
      const inspection = inspectTerminalPaste(text);
      if (inspection.requiresConfirmation) {
        wiring.reviewPaste(text);
        return;
      }
      paste(text);
    },
    clipboard,
    coarsePointer: () => live().touch,
    settings: () => live().clipboardSettings,
    refuse: () => wiring.problem(translate("terminal.clipboardRefused")),
    nothingToPaste: (image) => wiring.notice(translate(image ? "terminal.pasteImageUnsupported" : "terminal.pasteEmpty")),
    takePaste: (items) => kittyClipboard.takePaste(items),
    copied: () => wiring.notice(translate("copy.done")),
    enhancedKey: (event) => encodeIntlYen(event, live().jisYenBackslash),
    sendEnhancedKey: (sequence) => link.currentStream()?.send(sequence),
    selectionIsSearchMatch: wiring.search.isSelectingMatch,
  });
  const unregisterActions = registerTerminalActions(sessionId, {
    ...shortcutHandlers,
    terminalSearch: wiring.search.show,
    copy: terminalClipboard.copySelection,
    paste: terminalClipboard.paste,
    previousPrompt: () => prompts.jump(-1),
    nextPrompt: () => prompts.jump(1),
  });

  const stopObservingSize = observeTerminalSize(view, container, fitAndSync);
  const stopWatchingShown = watchTerminalShown(container, (shown) => link.currentStream()?.reportShown(shown));

  return {
    view,
    fit: fitAndSync,
    send,
    sendKey: (key) => send(keyBarInput(key, wiring.takeModifiers(), { applicationCursorKeys: view.modes.applicationCursorKeysMode })),
    paste,
    copy,
    prompts,
    promptCursor,
    commandDisplay,
    linkUnderlines,
    ligatureDrawing: parts.ligatureDrawing,
    statusLine,
    link,
    dispose: () => {
      disposed = true;
      link.close();
      stopObservingSize();
      stopWatchingShown();
      detachTouchScroll();
      scrollbackOffset.dispose();
      detachFontZoom();
      detachShortcuts();
      unregisterActions();
      terminalClipboard.detach();
      output.dispose();
      statusLine.dispose();
      wiring.statusLineHeight(null);
      detachPromptClicks();
      detachPromptJumpKeys();
      commandDisplay.dispose();
      wiring.closeCommandCard();
      wiring.topCommand(null);
      secretPromptWatch.dispose();
      wiring.cursorAtSecretPrompt(false);
      sequences.dispose();
      terminalLinks.dispose();
      linkUnderlines.dispose();
      unbindSearch();
      parts.dispose();
      copy.release();
    },
  };
}

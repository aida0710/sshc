import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { clipboard } from "../ui/clipboard";
import { bufferText } from "./buffer";
import { SelectSheet } from "./SelectSheet";
import { measuredCellHeight, newTouchScroll } from "./touchScroll";
import { KeyBar, applyModifiers, encodeKey, type Modifiers } from "./KeyBar";
import { openStream, type TerminalStream } from "./stream";
import { attachTerminalClipboard, type TerminalClipboardSettings } from "./clipboard";

type TerminalViewProps = {
  session: TerminalSession;
  api?: Pick<IntegrationsApi, "terminalStreamTicket">;
  // onExit は、子プロセスが終わったことを画面の他の部分へ伝える。一覧は
  // その行を終了済みとして描き直す。
  onExit?: () => void;
  copyOnSelect?: boolean;
  fontSize?: number;
  rightClickPaste?: boolean;
};

// Link は、この画面とセッションを繋ぐ通信路の状態である。
//
// **通信路が切れることは、セッションが死ぬことではない。** PTY は常駐プロセス
// 側で生きているので、新しいチケットを取れば同じセッションへ繋ぎ直せる。だから
// 切れたら黙って諦めず、繋ぎ直す。そして繋ぎ直していることを画面が言う——
// 何が起きているか分からないまま待たされる時間は、壊れているのと区別が付かない。
type Link =
  | { phase: "live" }
  | { phase: "connecting"; attempt: number }
  | { phase: "waiting"; attempt: number; seconds: number }
  // gone は「もう無いセッション」である。待っても戻ってこないので繋ぎ直さない。
  | { phase: "stopped"; gone: boolean };

// backoff は、繋ぎ直すまでに待つ秒数である。失敗した回数で選ぶ。
//
// 上限を置くのは、相手が同じ機械の中に居るからである。落ちているのは常駐
// プロセスか通信路のどちらかで、どちらも 15 秒より長く待って良くなるものではない。
const backoff = [1, 2, 4, 8, 15];

// settled は、「繋がっていた」と数える長さである。これより長く繋がっていた
// あとの切断は、前の切断の続きではなく新しい切断なので、待ち時間を数え直す。
const settled = 10_000;

// TerminalView は、セッションひとつを主画面に描く。
//
// xterm.js を使うのは、VT エミュレータを自前で書くのが論外だからである。zsh の
// zle は alt-screen、bracketed paste、カーソル位置指定を使う。
export function TerminalView({
  session,
  api = integrationsApi,
  onExit,
  copyOnSelect = true,
  fontSize,
  rightClickPaste = true,
}: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const host = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const clipboardSettings = useRef<TerminalClipboardSettings>({ copyOnSelect, rightClickPaste });
  // イベント配線は端末の寿命に一度だけ行い、設定値だけを差し替える。
  clipboardSettings.current = { copyOnSelect, rightClickPaste };
  const [problem, setProblem] = useState("");
  const [link, setLink] = useState<Link>({ phase: "connecting", attempt: 1 });
  // 繋ぎ直しの操作は効果の中に住んでいる。端末を作り直さずに押せるように、
  // 押せる形だけをここへ出す。
  const control = useRef<{ now: () => void; stop: () => void }>({ now: () => {}, stop: () => {} });

  // 画面上のキーで立てた修飾は、**打鍵の経路と同じ場所に住む。** バーの上に
  // 英字キーは無いので、Ctrl の次に来るのは常にシステムのキーボードからの
  // 一文字である——KeyBar の中に閉じ込めると、それに乗せられない。
  //
  // ref と state を並べて持つのは、onData の配線が端末の寿命に一度だけ行われる
  // からである。state だけだと、その配線は最初の値を握ったままになる。
  const [modifiers, setModifiers] = useState<Modifiers>({ ctrl: false, alt: false });
  // 選べる面に出している文字。null は閉じている。**開いた時点の写しである** ——
  // 開いている間も端末は動き続けるが、選んでいる最中に足元が動くのは困る。
  const [selecting, setSelecting] = useState<string | null>(null);
  const armed = useRef<Modifiers>(modifiers);
  armed.current = modifiers;
  // send は、打たれたものひとつを修飾ごと通す唯一の口である。
  const send = useRef<(label: string) => void>(() => {});

  // セッションが変わったら端末ごと作り直す。同じ DOM に別のスクロールバックを
  // 流し込むと、前のセッションの続きに見えてしまう。
  useEffect(() => {
    const container = host.current;
    if (container === null) return;

    const view = new Terminal({
      // 幅と高さは fit アドオンが決めるので、ここでは初期値だけを置く。
      cols: 80,
      rows: 24,
      convertEol: false,
      cursorBlink: session.exited === undefined,
      // Android には SF Mono も Menlo も無い。**ui-monospace はそこで何にも
      // 解決しないことがある**ので、その端末が実際に持っている等幅を並べる。
      fontFamily:
        'ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", monospace',
      // 設定された値があればそれが答えである。無いときだけ画面の幅で決める
      // ——**ここだけは媒体クエリを JS で読む。** xterm の字は寸法計算に入る
      // 値であって CSS で塗り替えられるものではないので、breakpoint では
      // 届かない。13px は指で持つ画面には小さすぎる。
      fontSize: fontSize ?? (window.matchMedia("(max-width: 767px)").matches ? 15 : 13),
      theme: terminalTheme(),
      // スクロールバックはサーバー側のリングバッファが正本である。ここでの値は
      // 再生されたバイト列を画面に保つための余地にすぎない。
      scrollback: 5000,
    });
    const fit = new FitAddon();
    view.loadAddon(fit);
    view.open(container);
    terminal.current = view;

    let stream: TerminalStream | null = null;
    let live = true;
    // stopped は人が再試行を止めたことである。飛んでいる要求が遅れて返って
    // きても、止めたあとに繋がってはならない。
    let stopped = false;
    let attempts = 0;
    let linkedAt = 0;
    let timer: ReturnType<typeof setInterval> | undefined;
    const decoder = new TextDecoder();

    // 変わっていない大きさは送らない。
    //
    // 窓を掴んで動かしているあいだ ResizeObserver は何度でも鳴るが、桁と行が
    // 変わるのはその何分の一かでしかない。同じ大きさを送り続けると、向こうの
    // シェルは鳴るたびに SIGWINCH を受けてプロンプトを描き直す——それが、
    // 掴んでいるあいだ画面が暴れる理由である。
    let sent = "";
    const syncSize = () => {
      const size = `${view.cols}x${view.rows}`;
      if (stream === null || view.cols === 0 || view.rows === 0 || size === sent) return;
      sent = size;
      stream.resize(view.cols, view.rows);
    };

    // 隠れているあいだは測らない。**面を離れても端末は mount したままにする**
    // ので、幅も高さも 0 になる瞬間がある。0 を測って送れば、向こうのシェルは
    // その大きさを信じる。
    const measure = () => {
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
      try {
        fit.fit();
      } catch {
        return;
      }
      syncSize();
    };
    measure();

    // 指で流す。**xterm はこれを持っていない**——スクロールする層は絶対配置で
    // 下に敷かれ、上に画面の層が乗っているので、指が触れるのは常に上である。
    //
    // preventDefault しない。止めれば長押しからの範囲選択も一緒に殺す。
    const scroll = newTouchScroll(view, () => measuredCellHeight(container, view.rows));
    // 指は 1 本のときだけ見る。2 本目は拡大か、この画面の外の操作である。
    const single = (event: TouchEvent): Touch | null =>
      event.touches.length === 1 ? (event.touches[0] ?? null) : null;
    const touchStart = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null) scroll.start(finger.clientY);
    };
    const touchMove = (event: TouchEvent) => {
      const finger = single(event);
      if (finger !== null) scroll.move(finger.clientY);
    };
    container.addEventListener("touchstart", touchStart, { passive: true });
    container.addEventListener("touchmove", touchMove, { passive: true });

    // 打鍵の配線はここで一度だけ行う。繋ぎ直すたびに足すと、1 回の打鍵が
    // 繋ぎ直した回数だけ PTY へ届く。
    //
    // **画面上のキーもここを通る。** 修飾が立っていれば、それが乗るのは次の
    // 一打鍵だけであり、乗った時点で降りる。押しっぱなしになる修飾は、次に
    // 打った一文字が何になるか分からない端末を作る。
    // **打たれた文字と、押されたキーは別のものである。** 前者はラベルの表を
    // 引いてはならない——"Esc" と打った人に ESC を送ることになる。後者は必ず
    // 引かなければならない——引かなければ、Esc のボタンが "Esc" という 3 文字を
    // 送る。1 つの入口で兼ねようとしたことが、まさにそれを起こした。
    const typed = (data: string) => {
      const { ctrl, alt } = armed.current;
      stream?.send(applyModifiers(data, ctrl, alt));
      if (ctrl || alt) setModifiers({ ctrl: false, alt: false });
    };
    send.current = (label: string) => {
      const { ctrl, alt } = armed.current;
      stream?.send(encodeKey(label, ctrl, alt));
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
          // 新しい通信路は、こちらの大きさを知らない。
          sent = "";
          linkedAt = Date.now();
          stream = openStream(issued.streamTicket, {
            onOutput: (chunk) => view.write(decoder.decode(chunk, { stream: true })),
            onExit: () => {
              view.options.cursorBlink = false;
              // 終わったセッションへは繋ぎ直さない。終わったことは切断ではない。
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
          if (session.exited === undefined) view.focus();
          syncSize();
        })
        .catch((error: unknown) => {
          if (!live || stopped) return;
          // もう無いセッションへは繋ぎ直さない。閉じられたか、常駐プロセスが
          // 入れ替わったかで、どちらも待って戻ってくるものではない。
          if (failureCode(error) === "terminal_session_not_found") {
            setLink({ phase: "stopped", gone: true });
            return;
          }
          retry();
        });
    };

    const retry = () => {
      if (!live || stopped) return;
      // しばらく繋がっていたなら、これは前の切断の続きではない。待ち時間を
      // 引きずると、一度荒れただけの通信路がその後ずっと 15 秒待ちになる。
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

    // コピーと貼り付けは自分で持つ。
    //
    // **xterm の選択はブラウザの選択ではない。** 選んだ範囲を知っているのは
    // xterm だけなので、Cmd+C をそのまま渡すと、ブラウザは空の選択を写して
    // 何も起きない——実際そうなっていた。写すのはここである。
    //
    // 割り当ては macOS が Cmd、それ以外が Ctrl+Shift である。**素の Ctrl+C は
    // 通す。** あれは SIGINT であり、端末が端末である理由のひとつである。
    const detachClipboard = attachTerminalClipboard({
      container,
      terminal: view,
      clipboard,
      settings: () => clipboardSettings.current,
      refuse: () => setProblem(t("terminal.clipboardRefused")),
    });

    // リサイズは TIOCSWINSZ を発行させる。これが無いと、全画面を使う
    // プログラム（vim、top、less）が壊れた幅で描画する。
    const observer = new ResizeObserver(measure);
    observer.observe(container);

    return () => {
      live = false;
      clearInterval(timer);
      observer.disconnect();
      container.removeEventListener("touchstart", touchStart);
      container.removeEventListener("touchmove", touchMove);
      detachClipboard();
      stream?.close();
      view.dispose();
      terminal.current = null;
    };
    // onExit と t はレンダーごとに新しくなりうる。ここが依存するのはセッション
    // だけであり、他が変わるたびに端末を作り直してはならない。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.id, api]);

  // テーマの切り替えは端末を作り直さない。色だけを読み直して差し替える。
  useEffect(() => {
    if (terminal.current === null) return;
    terminal.current.options.theme = terminalTheme();
  }, [resolved]);

  return (
    <section aria-label={t("terminal.screenLabel", { title: session.title })} className="relative flex min-h-0 flex-1 flex-col">
      {problem === "" ? null : (
        <p role="status" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {problem}
        </p>
      )}
      {/*
        繋がっているときは何も言わない。それ以外のときは、いま何をしているか、
        次に何が起きるか、そしてそれを変える手段を同じ行に置く。
      */}
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
        <p role="status" className="shrink-0 border-b border-line bg-card px-3 py-1.5 text-xs text-ink-muted">
          {session.exited.signal === ""
            ? t("terminal.exitedWithCode", { code: String(session.exited.code) })
            : t("terminal.exitedWithSignal", { signal: session.exited.signal })}
        </p>
      )}
      {/*
        端末の背景はトークンから来る。周りの面と同じ色にすると、どこまでが
        端末なのかが分からなくなる。
      */}
      <div ref={host} className="min-h-0 flex-1 bg-term-bg p-2" />
      {selecting === null ? null : (
        <SelectSheet text={selecting} onClose={() => setSelecting(null)} />
      )}
      <KeyBar
        modifiers={modifiers}
        onToggle={(name) => setModifiers((current) => ({ ...current, [name]: !current[name] }))}
        onKey={(label) => send.current(label)}
        onSelect={() => {
          const view = terminal.current;
          setSelecting(view === null ? "" : bufferText(view.buffer.active));
        }}
      />
    </section>
  );
}

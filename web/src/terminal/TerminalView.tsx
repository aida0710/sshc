import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { integrationsApi, type IntegrationsApi, type TerminalSession } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { useTheme } from "../theme/context";
import { terminalTheme } from "./theme";
import { openStream, type TerminalStream } from "./stream";

type TerminalViewProps = {
  session: TerminalSession;
  api?: Pick<IntegrationsApi, "terminalStreamTicket">;
  // onExit は、子プロセスが終わったことを画面の他の部分へ伝える。一覧は
  // その行を終了済みとして描き直す。
  onExit?: () => void;
};

// TerminalView は、セッションひとつを主画面に描く。
//
// xterm.js を使うのは、VT エミュレータを自前で書くのが論外だからである。zsh の
// zle は alt-screen、bracketed paste、カーソル位置指定を使う。
export function TerminalView({ session, api = integrationsApi, onExit }: TerminalViewProps) {
  const t = useTranslate();
  const { resolved } = useTheme();
  const host = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const [problem, setProblem] = useState("");

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
      fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace',
      fontSize: 13,
      theme: terminalTheme(),
      // スクロールバックはサーバー側のリングバッファが正本である。ここでの値は
      // 再生されたバイト列を画面に保つための余地にすぎない。
      scrollback: 5000,
    });
    const fit = new FitAddon();
    view.loadAddon(fit);
    view.open(container);
    fit.fit();
    terminal.current = view;

    let stream: TerminalStream | null = null;
    let live = true;
    const decoder = new TextDecoder();

    // コピーと貼り付けは自分で持つ。
    //
    // **xterm の選択はブラウザの選択ではない。** 選んだ範囲を知っているのは
    // xterm だけなので、Cmd+C をそのまま渡すと、ブラウザは空の選択を写して
    // 何も起きない——実際そうなっていた。写すのはここである。
    //
    // 割り当ては macOS が Cmd、それ以外が Ctrl+Shift である。**素の Ctrl+C は
    // 通す。** あれは SIGINT であり、端末が端末である理由のひとつである。
    view.attachCustomKeyEventHandler((event) => {
      if (event.type !== "keydown") return true;
      if (!event.metaKey && !(event.ctrlKey && event.shiftKey)) return true;
      const key = event.key.toLowerCase();
      if (key === "c" && view.hasSelection()) {
        void navigator.clipboard.writeText(view.getSelection());
        return false;
      }
      if (key === "v") {
        void navigator.clipboard
          .readText()
          .then((text) => {
            // 貼り付けはそのまま PTY へ流す。**ここで改行を解釈しない**——
            // 括弧付き貼り付けを求めるプログラムには、その旨が向こうで伝わる。
            if (text !== "") stream?.send(text);
          })
          .catch(() => setProblem(t("terminal.clipboardRefused")));
        return false;
      }
      return true;
    });

    void api
      .terminalStreamTicket(session.id)
      .then((issued) => {
        if (!live) return;
        stream = openStream(issued.streamTicket, {
          onOutput: (chunk) => view.write(decoder.decode(chunk, { stream: true })),
          onExit: () => {
            view.options.cursorBlink = false;
            onExit?.();
          },
          onClose: () => {
            // 通信路が切れてもセッションは死なない。同じ ID へ繋ぎ直せる。
            if (live) setProblem(t("terminal.disconnected"));
          },
        });
        if (session.exited === undefined) view.focus();
        // 打鍵はそのまま PTY へ。ここで解釈するものは何もない。
        view.onData((data) => stream?.send(data));
        stream.resize(view.cols, view.rows);
      })
      .catch(() => {
        if (live) setProblem(t("terminal.attachFailed"));
      });

    // リサイズは TIOCSWINSZ を発行させる。これが無いと、全画面を使う
    // プログラム（vim、top、less）が壊れた幅で描画する。
    const observer = new ResizeObserver(() => {
      try {
        fit.fit();
      } catch {
        return;
      }
      stream?.resize(view.cols, view.rows);
    });
    observer.observe(container);

    return () => {
      live = false;
      observer.disconnect();
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
    <section aria-label={t("terminal.screenLabel", { title: session.title })} className="flex min-h-0 flex-1 flex-col">
      {problem === "" ? null : (
        <p role="status" className="shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink">
          {problem}
        </p>
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
    </section>
  );
}

import { useRef, useState, type RefObject } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { composerSendBlock, planComposerSend, type ComposerSendBlock } from "./composerSend";
import type { StreamLink } from "./streamLink";
import type { ComposerField } from "./TerminalComposer";

const composerSendBlockMessages: Record<ComposerSendBlock, MessageKey> = {
  "wait-for-connection": "terminal.composerWaitForConnection",
  "reconnect-first": "terminal.composerReconnectFirst",
  "session-ended": "terminal.composerSessionEnded",
};

// TerminalComposerState は、スマホの入力欄（TerminalComposer）に打ったものと、その送り方である。
export type TerminalComposerState = {
  text: string;
  setText: (text: string) => void;
  // secret は、ターミナルがパスワードを聞いているあいだに打った答えである。聞いていなければ null。
  // この状態にだけ置き、打ったとおりに送る。
  secret: string | null;
  setSecret: (secret: string) => void;
  // sendable は、送るボタンが何かをするかである。false のあいだも submit は走り、送れない理由を知らせる。
  sendable: boolean;
  // placeholder は、送ることが文字を送る以外のこと（つなぎ直す）をするときの案内である。
  placeholder: string | undefined;
  submit: () => void;
  // fieldRef は、打鍵を受ける欄が出ているあいだ、その欄を指す。
  fieldRef: RefObject<ComposerField | null>;
  // setCursorAtSecretPrompt は、ターミナルのカーソルがパスワードの問いにあるかを伝える。
  setCursorAtSecretPrompt: (asking: boolean) => void;
  forgetSecret: () => void;
};

// useTerminalComposer は、スマホの入力欄の中身を持ち、送るときに何をするかを決める。
export function useTerminalComposer({ session, link, reconnect, bracketedPaste, sendInput, sendPaste, confirmPaste, notice }: {
  session: TerminalSession;
  link: StreamLink;
  // reconnect は、エンジンの行が Enter でつなぎ直すと言っているあいだの、つなぎ直す操作である。それ以外は null。
  reconnect: (() => void) | null;
  // bracketedPaste は、ターミナルのプログラムが bracketed paste を有効にしているかを返す。
  bracketedPaste: () => boolean;
  // sendInput は、打ったものとして送る。sendPaste は、貼り付けとして送る。
  sendInput: (text: string) => void;
  sendPaste: (text: string) => void;
  // confirmPaste は、貼り付けなら確かめる文字（改行、制御文字）を、送る前に確かめてもらう。
  confirmPaste: (text: string) => void;
  notice: (message: string) => void;
}): TerminalComposerState {
  const t = useTranslate();
  const [text, setText] = useState("");
  const [typedSecret, setTypedSecret] = useState("");
  const [cursorAtSecretPrompt, setCursorAtSecretPrompt] = useState(false);
  const fieldRef = useRef<ComposerField>(null);

  const sendBlock = composerSendBlock(session, link);
  // 終わったセッションの画面に残った問いは、何も待っていない。つなごうとしているだけのリンクでは、欄を
  // 伏せたまま打ったものを持っておく。戻ったときに問いはまだそこにある。
  const masked = cursorAtSecretPrompt && sendBlock !== "reconnect-first" && sendBlock !== "session-ended";
  // 問いへの答えは問いと一緒に終わるので、伏せずに出ることが無い。
  if (!masked && typedSecret !== "") setTypedSecret("");

  function submit() {
    // 欄に打ったものは残す。つなぎ直したシェルへ送るためである。
    if (reconnect !== null) {
      reconnect();
      return;
    }
    if (sendBlock !== null) {
      notice(t(composerSendBlockMessages[sendBlock]));
      return;
    }
    if (masked) {
      // 打ったものとして送る。貼り付けは bracketed になることがあり、パスワードの問いはそれもパスワードの
      // 一部として受け取る。危ない貼り付けを確かめるダイアログに出すと、答えが見えてしまう。
      sendInput(`${typedSecret}\r`);
      setTypedSecret("");
      return;
    }
    const plan = planComposerSend(text, { bracketedPaste: bracketedPaste() });
    if (plan.kind === "confirm") {
      confirmPaste(plan.text);
      return;
    }
    if (plan.kind === "paste") sendPaste(plan.text);
    sendInput("\r");
    setText("");
  }

  return {
    text,
    setText,
    secret: masked ? typedSecret : null,
    setSecret: setTypedSecret,
    sendable: sendBlock === null || reconnect !== null,
    placeholder: reconnect === null ? undefined : t("terminal.composerReconnectPlaceholder"),
    submit,
    fieldRef,
    setCursorAtSecretPrompt,
    forgetSecret: () => setTypedSecret(""),
  };
}

import type { Translate } from "./context";
import type { MessageKey } from "./messages";

// CodeMessages は、engine が返す機械向けの値（problem code、検査の結果など）から、
// 画面に出す文の key を引く表である。
export type CodeMessages = Readonly<Record<string, MessageKey>>;

// CodeTextLookup は、code を引く表と、表に無いときの画面ごとの一般的な文の組である。
export type CodeTextLookup = {
  messages: CodeMessages;
  fallback: MessageKey;
};

// codeText は、engine の値を画面に出す1文にする。code をそのまま見せても、利用者には
// 何が起きたか分からない。表に無い値と、code を持たない失敗（fetch の失敗など）は、
// fallback の文に落とす。
export function codeText(t: Translate, code: string, { messages, fallback }: CodeTextLookup): string {
  return t(Object.hasOwn(messages, code) ? messages[code]! : fallback);
}

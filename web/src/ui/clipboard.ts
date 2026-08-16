// このアプリケーションがクリップボードへ書く唯一の口。
//
// **navigator.clipboard だけでは足りない。** Android の WebView はページが
// secure context であってもこれを拒むことがあり、実機ではまさに端末の
// 「クリップボードにアクセスできませんでした」がそれで出ていた。読み取りには
// 代わりの道が無い（clipboard-read の許可を WebView は与えない）が、書き込みには
// 選択と execCommand という古い道がまだ残っている。
//
// **執行猶予付きの API を使うのは、これが最後の砦だからである。** 新しい方を
// 先に試し、断られたときだけ古い方へ落ちる。どちらも駄目なら、呼び出し側が
// 失敗を伝える。

export type ClipboardAccess = { readText(): Promise<string>; writeText(text: string): Promise<void> };

// execCommandCopy は、画面外の textarea を選択して copy を実行する。
//
// 見えない要素は選択できないので、隠すのではなく画面の外へ出す。読み上げには
// aria-hidden で伏せる——これは人へ向けた要素ではない。
function execCommandCopy(text: string): boolean {
  const holder = document.createElement("textarea");
  holder.value = text;
  holder.setAttribute("readonly", "");
  holder.setAttribute("aria-hidden", "true");
  holder.style.position = "fixed";
  holder.style.top = "-1000px";
  holder.style.opacity = "0";
  document.body.appendChild(holder);
  try {
    holder.select();
    // eslint に見せる設定は無いが、これが猶予付きであることは承知の上である。
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    holder.remove();
  }
}

export const clipboard: ClipboardAccess = {
  async readText() {
    if (navigator.clipboard === undefined) throw new Error("clipboard_unavailable");
    return navigator.clipboard.readText();
  },
  async writeText(text: string) {
    try {
      if (navigator.clipboard !== undefined) {
        await navigator.clipboard.writeText(text);
        return;
      }
    } catch {
      // 断られた。古い道がまだ残っている。
    }
    if (!execCommandCopy(text)) throw new Error("clipboard_refused");
  },
};

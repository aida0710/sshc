// Android のソフトキーボードから打った文字が重複する問題を止める。
//
// **xterm の Android 経路は競合している。** 確定した 1 文字ごとに、Android の
// IME は keyCode 229 の keydown を送る。xterm はそれを見て、こう振る舞う。
//
//     _handleAnyTextareaChanges() {
//       const before = this._textarea.value;
//       setTimeout(() => {
//         const after = this._textarea.value;
//         this._coreService.triggerDataEvent(after.replace(before, ""), true);
//       }, 0);
//     }
//
// **textarea は消されない。** だからこれは「次の打鍵が来る前に setTimeout(0)
// が解決する」ことに賭けている。速く打てば——予測変換の確定でも、貼り付けでも
// ——4 つの timeout がどれも最終値 "echo" を見て、`replace("")` が "echo"、
// `replace("e")` が "cho"、`replace("ec")` が "ho" を返す。**打った 4 文字が
// 10 文字になって PTY へ届く。** 実機のエミュレータで測って、そうなっていた。
//
// これは xtermjs/xterm.js#5108 として報告されている。
//
// **直し方は、その keydown を xterm に見せないことである。** 見なければ
// _keyDownSeen が立たず、xterm は input イベントの側で処理する——あちらは
// event.data をそのまま送るだけで、競合しない。
//
// 上流が直したら、これは丸ごと消える。

/** Android の IME が「これは IME が処理中」を意味するのに使う keyCode。 */
export const imeKeyCode = 229;

/**
 * withholdFromTerminal は、その keydown を xterm に渡さないかを答える。
 *
 * <p>**変換の最中は渡す。** 日本語の入力は同じ 229 を使うが、そちらは
 * compositionstart から compositionend までの別の経路で組み立てられており、
 * xterm はその区間を自分で数えている。取り上げれば、その数えが合わなくなる。
 */
export function withholdFromTerminal(keyCode: number, composing: boolean): boolean {
  return keyCode === imeKeyCode && !composing;
}

type Attachment = {
  /** xterm より外側の要素。**capture はここでしか先に取れない。** */
  container: HTMLElement;
  /** xterm が作る隠しの入力欄。変換の開始と終了はここに来る。 */
  textarea: HTMLElement;
};

export function attachImeKeys({ container, textarea }: Attachment): () => void {
  let composing = false;
  const began = () => {
    composing = true;
  };
  const ended = () => {
    composing = false;
  };
  const withhold = (event: KeyboardEvent) => {
    if (withholdFromTerminal(event.keyCode, composing)) event.stopPropagation();
  };

  textarea.addEventListener("compositionstart", began, true);
  textarea.addEventListener("compositionend", ended, true);
  // **祖先の capture である。** 同じ要素に足しても、先に登録された xterm の
  // listener より後に走る——目的は先回りすることなので、そこでは届かない。
  container.addEventListener("keydown", withhold, true);

  return () => {
    textarea.removeEventListener("compositionstart", began, true);
    textarea.removeEventListener("compositionend", ended, true);
    container.removeEventListener("keydown", withhold, true);
  };
}

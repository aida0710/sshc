// 触れる画面では、範囲選択を OS に返す。
//
// **xterm はタッチを持っていない。** これは私たちの配線の穴ではなく、
// ライブラリの穴である——xtermjs/xterm.js#3727 (2022) と #5377 (2025) が
// どちらも開いたままで、CoreBrowserTerminal はマウスとキーボードしか見ていない。
//
// 選択が始まらない理由は 1 行に絞れる。xterm は自分の mousedown で**無条件に**
// preventDefault を呼んでおり、長押しが生む mousedown もそこで潰れる。
// user-select を CSS でどう戻しても、ブラウザは選択を開始できない。
//
// 上流も同じ結論に達している。xtermjs/xterm.js#5961 は「粗いポインタでは
// xterm の処理を止めて OS の DOM 選択に任せる」を実装しており、maintainer は
// 方針を認めた上で「Android にも対応しろ」と要求している。**その Android 分は
// まだ誰も書いていない。** ここにあるのは、それを外から当てたものである。
//
// 上流がマージされたら、これは消して nativeSelection: 'auto' に置き換わる。

/** 指で触る画面か。hover が無く、ポインタが粗いことがその定義である。 */
export function prefersNativeSelection(match: (query: string) => { matches: boolean }): boolean {
  return match("(hover: none) and (pointer: coarse)").matches;
}

// タップと長押しを分ける閾値。**長押しは選択であって、焦点の移動ではない。**
export const tapMillis = 400;
export const tapSlopPixels = 10;

/** isTap は、その指の動きが「叩いた」なのか「押さえた」なのかを答える。 */
export function isTap(millis: number, movedPixels: number): boolean {
  return millis >= 0 && millis <= tapMillis && movedPixels <= tapSlopPixels;
}

/** 画面のどこかに範囲が選ばれているか。選ばれている間は指で流さない。 */
export function hasSelection(selection: Selection | null): boolean {
  return selection !== null && !selection.isCollapsed && selection.toString() !== "";
}

export const nativeSelectionClass = "sshc-native-selection";

type Attachment = {
  container: HTMLElement;
  focus: () => void;
  now: () => number;
};

/**
 * attachNativeSelection は、xterm に mousedown を見せず、焦点を自分で配る。
 *
 * <p>止めた以上、焦点はこちらの仕事になる——xterm はそれを mousedown の中で
 * やっていた。**配るのはタップのときだけである。** 長押しは選択であり、そこで
 * 焦点を動かせば OS が掴んだ範囲が外れる。
 */
export function attachNativeSelection({ container, focus, now }: Attachment): () => void {
  container.classList.add(nativeSelectionClass);

  const swallow = (event: MouseEvent) => event.stopPropagation();
  container.addEventListener("mousedown", swallow, { capture: true });

  let startedAt = 0;
  let startX = 0;
  let startY = 0;
  const began = (event: TouchEvent) => {
    const finger = event.touches[0];
    if (finger === undefined) return;
    startedAt = now();
    startX = finger.clientX;
    startY = finger.clientY;
  };
  const ended = (event: TouchEvent) => {
    const finger = event.changedTouches[0];
    if (finger === undefined) return;
    const moved = Math.hypot(finger.clientX - startX, finger.clientY - startY);
    if (isTap(now() - startedAt, moved)) focus();
  };
  container.addEventListener("touchstart", began, { passive: true });
  container.addEventListener("touchend", ended, { passive: true });

  return () => {
    container.classList.remove(nativeSelectionClass);
    container.removeEventListener("mousedown", swallow, { capture: true });
    container.removeEventListener("touchstart", began);
    container.removeEventListener("touchend", ended);
  };
}

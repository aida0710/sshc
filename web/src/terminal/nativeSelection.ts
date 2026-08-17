// 触れる画面かどうかを答える、それだけの場所。
//
// **かつてここには「何を戻せば選択が始まるか」が書いてあった。** 全部間違って
// いた——user-select も pointer-events も touch-action も aria-hidden も焦点も
// mousedown の遮断も contextmenu の遮断も、実機で 1 つずつ測って全部空振り
// だった。`.xterm` の中では、指の長押しから選択が始まらない。同じ普通の div を
// 中に置くと選べず、外に置くと選べる。原因は xterm がその部分木に対して行って
// いる何かで、外からは外せない。
//
// 答えは selectionOverlay.ts にある——字を `.xterm` の外へ出す。ここに残って
// いるのは、それをどの画面で行うかを決める 1 行だけである。

/** 指で触る画面か。hover が無く、ポインタが粗いことがその定義である。 */
export function prefersNativeSelection(match: (query: string) => { matches: boolean }): boolean {
  return match("(hover: none) and (pointer: coarse)").matches;
}

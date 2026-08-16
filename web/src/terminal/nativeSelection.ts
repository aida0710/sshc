// 触れる画面では、範囲選択を OS に返す。
//
// **xterm はタッチを持っていない。** これは私たちの配線の穴ではなくライブラリの
// 穴である——xtermjs/xterm.js#3727 (2022) と #5377 (2025) がどちらも開いたままで、
// CoreBrowserTerminal はマウスとキーボードしか見ていない。上流の #5961 が
// 「粗いポインタでは OS の DOM 選択に任せる」を実装しているが、iOS 向けで
// 未マージであり、maintainer が求めている Android 分はまだ誰も書いていない。
//
// **止めるものは 2 つだけだった。** 実機の WebView を DevTools で覗いて分かった
// ことである。
//
//  1. xterm は .xterm-rows に pointer-events: none を掛けている。だから長押しの
//     当たり先は .xterm-screen になり、そこに文字は無い——選択は始まるのに
//     掴むものが無く、空のまま終わる。
//  2. 右クリック貼り付けが contextmenu を preventDefault していた。Android の
//     長押しは contextmenu を発火するので、そこで既定を止めると選択ジェスチャ
//     ごと消える。ついでに読み取れないクリップボードを読みに行き、「アクセス
//     できませんでした」を出していた。
//
// **mousedown は関係なかった。** 長押しでは 1 度も発火しない——計測した。

/** 指で触る画面か。hover が無く、ポインタが粗いことがその定義である。 */
export function prefersNativeSelection(match: (query: string) => { matches: boolean }): boolean {
  return match("(hover: none) and (pointer: coarse)").matches;
}

export const nativeSelectionClass = "sshc-native-selection";

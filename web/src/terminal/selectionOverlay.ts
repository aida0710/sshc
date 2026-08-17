import { viewportText, type ViewportBuffer } from "./buffer";

// 端末の上に、選べる文字を重ねる。
//
// **`.xterm` の中では、指の長押しから範囲選択が始まらない。** 実機に DevTools を
// 繋いで、同じ普通の div を `.xterm` の中と `body` の下に置いて比べた——中では
// 選べず、外では選べる。CSS でも contenteditable でも touch-action でもない。
// 止めている候補は 1 つずつ潰した: user-select、pointer-events、touch-action、
// aria-hidden、焦点、mousedown の遮断、xterm の contextmenu ハンドラの遮断、
// 私たち自身の contextmenu の遮断、blur 経由の再描画、screen reader の木。
// **どれでもなかった。** 原因は xterm がその部分木に対して行っている何かで、
// 外からは外せない。
//
// だから文字を `.xterm` の外へ出す。**この板は `.xterm` の兄弟である**——親を
// 一段またぐだけで、OS の長押しも、ハンドルも、コピーの吹き出しも戻ってくる。
// 板の字は透明で、見えているのは下の xterm の字である。板から出るのは帯だけだ。

export const overlayClass = "sshc-select-overlay";

// 指がぶれる幅。これを超えたら「触った」ではなく「引いた」である。
const tapSlopPixels = 8;
// Android の長押しは 500ms 前後で成立する。そこへ近づいたものは打鍵ではなく、
// 選ぶための操作である。
const tapHoldMillis = 400;

type OverlayTerminal = {
  readonly element: HTMLElement | undefined;
  readonly rows: number;
  readonly cols: number;
  readonly buffer: { readonly active: ViewportBuffer };
  focus(): void;
  blur(): void;
  onRender(handler: () => void): { dispose(): void };
};

/** selectionHeldIn は、その中に**掴まれたままの選択**があるかを答える。 */
export function selectionHeldIn(node: HTMLElement): boolean {
  const selection = node.ownerDocument.getSelection();
  if (selection === null || selection.isCollapsed) return false;
  return node.contains(selection.anchorNode);
}

export function attachSelectionOverlay(container: HTMLElement, view: OverlayTerminal): () => void {
  const overlay = container.ownerDocument.createElement("pre");
  overlay.className = `${overlayClass} absolute m-0 select-text overflow-hidden whitespace-pre text-transparent`;
  // 読み上げには xterm 自身の木がある。同じ文字を二度読ませない。
  overlay.setAttribute("aria-hidden", "true");
  overlay.dir = "ltr";
  // **`.xterm-helpers` より上に居る。** あれは z-index 5 で、カーソルの位置へ
  // 動かされる隠しの textarea を抱えている——下に居ると、カーソルの周りだけ
  // 長押しがその空の入力欄を掴む。
  overlay.style.zIndex = "6";
  container.appendChild(overlay);

  let laidOut = "";

  const paint = () => {
    if (view.rows <= 0 || view.cols <= 0) return;

    const screen = view.element?.querySelector<HTMLElement>(".xterm-screen") ?? null;
    const rows = view.element?.querySelector<HTMLElement>(".xterm-rows") ?? null;
    const box = screen?.getBoundingClientRect() ?? null;
    const base = container.getBoundingClientRect();
    const measurable = screen !== null && rows !== null && box !== null && box.width > 0 && box.height > 0;

    // **書くのは変わったときだけである。** 毎フレーム style を触れば、その
    // たびにレイアウトをやり直させることになる。
    const shape = measurable
      ? `${box.left - base.left} ${box.top - base.top} ${box.width} ${box.height}`
      : laidOut;
    const relaidOut = shape !== laidOut;

    // **形が変わったなら、掴んでいたものはもう合わない。**
    //
    // キーボードが閉じれば窓の高さが変わり、xterm は桁と行を測り直して全部を
    // 描き直す。板の字だけを止めておくと、帯は動いた字の上に残る——選んだ
    // つもりの範囲と、見えている範囲が食い違う。実機でそうなった。
    // 合わなくなった選択は、持っていても嘘なので手放す。
    if (relaidOut) overlay.ownerDocument.getSelection()?.removeAllRanges();

    // **選んでいる最中は写さない。** textContent を差し替えれば選択は消え、
    // ハンドルもコピーの吹き出しも一緒に消える。裏で流れ続ける出力より、
    // いま指が掴んでいるものの方が新しい。
    if (relaidOut || !selectionHeldIn(overlay)) {
      overlay.textContent = viewportText(view.buffer.active, view.rows, view.cols);
    }

    if (!measurable || !relaidOut) return;
    laidOut = shape;

    overlay.style.left = `${box.left - base.left}px`;
    overlay.style.top = `${box.top - base.top}px`;
    overlay.style.width = `${box.width}px`;
    overlay.style.height = `${box.height}px`;
    // **寸法は xterm から写すのであって、計算し直すのではない。**
    // `.xterm-screen` の幅は xterm が css.canvas.width として自分で書いた値で、
    // 1 マスの幅はそれを桁数で割ったものである。ここで割れば同じ数が出る。
    overlay.style.lineHeight = `${box.height / view.rows}px`;
    // **字の送りを決めているのは font ではなく letter-spacing である。**
    // xterm は「1 マスの幅 − 実測した W の幅」をそこへ入れ、端末が実際に解決した
    // 等幅がどれであっても桁が揃うようにしている。読めば、その較正ごと写せる。
    // 自分で測り直せば必ずずれる。
    const style = getComputedStyle(rows);
    overlay.style.fontFamily = style.fontFamily;
    overlay.style.fontSize = style.fontSize;
    overlay.style.fontWeight = style.fontWeight;
    overlay.style.letterSpacing = style.letterSpacing;
    overlay.style.fontKerning = "none";
  };

  // **軽く触ったら、打つつもりである。**
  //
  // 板が上に乗ったので xterm はもう指を見ない。焦点を渡すのはこちらの仕事に
  // なる——渡さなければ、触れる画面でソフトキーボードが二度と開かない。
  //
  // 引いたあとには渡さない。スクロールのたびにキーボードが開けば、画面の半分が
  // その場で消える。長押しのあとにも渡さない。焦点を動かせば、OS がいま出した
  // ばかりのハンドルと吹き出しが消える。
  let touchedAt = 0;
  let touchedY = 0;
  let dragged = false;
  const began = (event: TouchEvent) => {
    const finger = event.touches[0];
    touchedAt = finger === undefined ? 0 : Date.now();
    touchedY = finger?.clientY ?? 0;
    dragged = false;
  };
  const moved = (event: TouchEvent) => {
    const finger = event.touches[0];
    if (finger !== undefined && Math.abs(finger.clientY - touchedY) > tapSlopPixels) dragged = true;
  };
  const ended = () => {
    const tapped = !dragged && touchedAt !== 0 && Date.now() - touchedAt < tapHoldMillis;
    touchedAt = 0;
    if (!tapped) return;
    // 選んだものは、次に触れた時点で用済みである。残したままだと、次の描き直しが
    // 止まったままになる。
    overlay.ownerDocument.getSelection()?.removeAllRanges();
    // **一度外してから当てる。** 焦点が既にそこにあると、Android はキーボードを
    // 出し直さない——選択のあいだに閉じたものが、叩いても二度と開かなくなる。
    view.blur();
    view.focus();
  };
  // **触った指の後から来る mousedown を、既定のまま通さない。**
  //
  // Chromium は touchend のあとに mousedown / mouseup / click を投げる。その
  // mousedown の既定動作は、焦点を「押された要素の、焦点を取れる祖先」へ移す
  // ことである——板は取れないので、行き先は body になる。touchend で当てた
  // ばかりの textarea はそこで外れ、キーボードは上がらない。
  //
  // 実機に DevTools を繋いで測った並びがこれである:
  //   touchstart > touchend > focusin(textarea) > mousedown > focusout(textarea)
  // preventDefault を入れると focusout が消え、mInputShown が true になった。
  //
  // 長押しからの選択は壊れない。あちらは touch のジェスチャとして解決され、
  // 選択が始まった指には互換 mouse イベントがそもそも来ない——入れたまま
  // 長押しして "com" が選べることを実機で確かめた。
  const swallowCompatMouse = (event: MouseEvent) => event.preventDefault();
  overlay.addEventListener("touchstart", began, { passive: true });
  overlay.addEventListener("touchmove", moved, { passive: true });
  overlay.addEventListener("touchend", ended, { passive: true });
  overlay.addEventListener("mousedown", swallowCompatMouse);

  // **onRender だけで足りる。** 書き込みも、流したことも、大きさが変わったことも、
  // 画面に出るときは必ずここを通る。点滅だけの描き直しでは鳴らず、1 フレームに
  // 1 度へ間引くのも xterm が済ませている。
  const render = view.onRender(paint);
  paint();

  return () => {
    render.dispose();
    overlay.remove();
  };
}

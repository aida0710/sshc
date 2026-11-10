import { flushSync } from "react-dom";
import { parseSectionPath } from "../routing/sectionRoute";
import type { BrowserLocation } from "../routing/useSectionRoute";
import { matchesMediaQuery, mobileViewportQuery } from "./useMediaQuery";
import { drawPageWhole } from "./backGesturePage";

// Android の戻るジェスチャーのあとにページがフェードしている間（pageMotion.css）、<html> にこの属性が付く。
const pageChangingAttribute = "data-page-changing";
// 切り替えのたびに数を増やす。次の切り替えで打ち切られたフェードが終わっても、次の切り替えの印を消さないため。
let changeCount = 0;

function movesOnThisScreen(): boolean {
  return typeof document.startViewTransition === "function"
    && document.documentElement.dataset.motion !== "reduce"
    && matchesMediaQuery(mobileViewportQuery);
}

// backGestureLeftPageSmall は、Android の戻るジェスチャーがページを小さく描いたまま（backGesturePage.ts）指が
// 離れたかどうかを返す。このあと、戻るで前のページを出す。
function backGestureLeftPageSmall(): boolean {
  return document.documentElement.dataset.backGesture !== undefined;
}

function sectionOf(location: BrowserLocation) {
  const route = parseSectionPath(location.pathname);
  return route.kind === "section" ? route.section : null;
}

// presentLocationChange は、場所が開くページを出す（change が新しい場所を知らせる）。タブ・リンク・戻るボタンでは、
// すぐにページを変える。フェードすると、その間は画面が前後の絵で覆われ、押してもページに届かないため。電話で
// Android の戻るジェスチャーを離したときだけ、指の下で小さくなったページから前のページへフェードする。小さい
// ページが急に元の大きさのページへ変わらないようにするため。ブラウザが前後の画面の絵を撮り、pageMotion.css が
// 一方から他方へフェードする。広い画面、view transition の無いブラウザ、動きを減らす設定では、すぐに変える。
//
// ページが自分の区画の中で場所を整える置き換え（Connections がサーバの一覧で開く）には、新しく見せるものが無い。
// フェードせず、ページを開いたフェードも打ち切らない。別の区画を開く置き換え（Android の戻るで Home を開く）は、
// ほかと同じページの切り替えとして扱う。
export function presentLocationChange(
  change: () => void,
  { from, to, replaced }: { from: BrowserLocation; to: BrowserLocation; replaced: boolean },
) {
  const samePage = from.pathname === to.pathname && from.search === to.search;
  const settlingAddress = replaced && sectionOf(from) === sectionOf(to);
  if (samePage || settlingAddress || !backGestureLeftPageSmall() || !movesOnThisScreen()) {
    change();
    return;
  }
  const root = document.documentElement;
  const thisChange = ++changeCount;
  root.setAttribute(pageChangingAttribute, "");
  const transition = document.startViewTransition(() => {
    // 切り替える前の絵ではページは小さいまま。切り替えたあとのページは元の大きさで描く。
    drawPageWhole();
    flushSync(change);
  });
  void transition.finished.finally(() => {
    if (changeCount === thisChange) root.removeAttribute(pageChangingAttribute);
  });
}

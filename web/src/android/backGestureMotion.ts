import { useEffect } from "react";
import { drawBackGesture, drawPageWhole, settleBackGesture } from "../ui/backGesturePage";
import { matchesMediaQuery, mobileViewportQuery } from "../ui/useMediaQuery";

// What the Android app sends while the person swipes Back (MainActivity): the gesture starts, follows the finger
// (progress from 0 to 1, from the left or the right edge), is let go to go back (commit), or is taken back (cancel).
type BackGesture = { phase: "start" | "progress" | "commit" | "cancel"; progress: number; edge: "left" | "right" };

const backGestureEvent = "sshc-android-back-gesture";

// useBackGestureMotion は、Android が自分の画面に描くのと同じように、戻るジェスチャーをページに描く。指が動く間、
// ページは少し小さくなり、指が来た端から離れる（backGesturePage.ts）。離すと短いフェードで前のページへ戻り
// （pageMotion.ts がページを元の大きさに描き直す）、取り消すと元の位置へ大きくなって戻る。backMovesPage は、
// 戻るで別のページへ移るかを返す。戻るがダイアログやメニューを閉じるだけのときや、アプリを出るときは、ページを
// 小さくしない。
export function useBackGestureMotion(backMovesPage: () => boolean) {
  useEffect(() => {
    const root = document.documentElement;
    let drawing = false;
    const follow = (event: Event) => {
      const gesture = (event as CustomEvent<BackGesture>).detail;
      if (gesture.phase === "start") {
        drawing = backMovesPage() && matchesMediaQuery(mobileViewportQuery) && root.dataset.motion !== "reduce";
        if (drawing) drawBackGesture(gesture.progress, gesture.edge);
        return;
      }
      if (!drawing) return;
      if (gesture.phase === "progress") {
        drawBackGesture(gesture.progress, gesture.edge);
        return;
      }
      drawing = false;
      if (gesture.phase === "cancel") {
        settleBackGesture();
        return;
      }
      // Let go, Back goes to the page before, and the fade to it draws the page whole. A Back that turns out to go
      // nowhere (an unsaved draft asks first) leaves the page small; it grows back once the frame after it is drawn.
      window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
        if (root.dataset.backGesture !== undefined) settleBackGesture();
      }));
    };
    window.addEventListener(backGestureEvent, follow);
    return () => {
      window.removeEventListener(backGestureEvent, follow);
      drawPageWhole();
    };
  }, [backMovesPage]);
}

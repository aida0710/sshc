import { useEffect } from "react";
import { drawBackGesture, drawPageWhole, settleBackGesture } from "../ui/backGesturePage";
import { matchesMediaQuery, mobileViewportQuery } from "../ui/useMediaQuery";

// What the Android app sends while the person swipes Back (MainActivity): the gesture starts, follows the finger
// (progress from 0 to 1, from the left or the right edge), is let go to go back (commit), or is taken back (cancel).
type BackGesture = { phase: "start" | "progress" | "commit" | "cancel"; progress: number; edge: "left" | "right" };

const backGestureEvent = "sshc-android-back-gesture";

// useBackGestureMotion draws Android's back gesture on the page as Android draws it for its own screens: while the
// finger moves, the page shrinks a little and moves away from the edge it came from (backGesturePage.ts). Let go,
// the page goes back with a short fade (pageMotion.ts draws the page whole again); taken back, it grows back to
// its place. backMovesPage says whether Back would go to another page: it does not shrink a page whose Back only
// closes a dialog or a menu, or leaves the app.
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

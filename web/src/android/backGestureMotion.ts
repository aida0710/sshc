import { useEffect } from "react";
import { matchesMediaQuery, mobileViewportQuery } from "../ui/useMediaQuery";

// What the Android app sends while the person swipes Back (MainActivity): the gesture starts, follows the finger
// (progress from 0 to 1, from the left or the right edge), is let go to go back (commit), or is taken back (cancel).
type BackGesture = { phase: "start" | "progress" | "commit" | "cancel"; progress: number; edge: "left" | "right" };

const backGestureEvent = "sshc-android-back-gesture";
// How long the page takes to grow back to its place when the gesture is taken back (pageMotion.css).
const settleMilliseconds = 200;

function drawProgress(root: HTMLElement, { progress, edge }: BackGesture) {
  root.style.setProperty("--sshc-back-progress", String(Math.min(1, Math.max(0, progress))));
  root.style.setProperty("--sshc-back-edge", edge === "left" ? "1" : "-1");
}

function forgetGesture(root: HTMLElement) {
  delete root.dataset.backGesture;
  root.style.removeProperty("--sshc-back-progress");
  root.style.removeProperty("--sshc-back-edge");
}

// useBackGestureMotion draws Android's back gesture on the page as Android draws it for its own screens: while the
// finger moves, the page shrinks a little and moves away from the edge it came from (pageMotion.css). Let go, the
// page goes back with its usual fade (pageMotion.ts draws the page whole again); taken back, it grows back to its
// place. backMovesPage says whether Back would go to another page: it does not shrink a page whose Back only
// closes a dialog or a menu, or leaves the app.
export function useBackGestureMotion(backMovesPage: () => boolean) {
  useEffect(() => {
    const root = document.documentElement;
    let drawing = false;
    let settleTimer: number | undefined;
    const settle = () => {
      root.dataset.backGesture = "settling";
      root.style.setProperty("--sshc-back-progress", "0");
      window.clearTimeout(settleTimer);
      settleTimer = window.setTimeout(() => forgetGesture(root), settleMilliseconds);
    };
    const follow = (event: Event) => {
      const gesture = (event as CustomEvent<BackGesture>).detail;
      if (gesture.phase === "start") {
        window.clearTimeout(settleTimer);
        drawing = backMovesPage() && matchesMediaQuery(mobileViewportQuery) && root.dataset.motion !== "reduce";
        if (!drawing) return;
        root.dataset.backGesture = "";
        drawProgress(root, gesture);
        return;
      }
      if (!drawing) return;
      if (gesture.phase === "progress") {
        drawProgress(root, gesture);
        return;
      }
      drawing = false;
      if (gesture.phase === "cancel") {
        settle();
        return;
      }
      // Let go, Back goes to the page before, and the fade to it draws the page whole. A Back that turns out to go
      // nowhere (an unsaved draft asks first) leaves the page small; it grows back once the frame after it is drawn.
      window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
        if (root.dataset.backGesture !== undefined) settle();
      }));
    };
    window.addEventListener(backGestureEvent, follow);
    return () => {
      window.removeEventListener(backGestureEvent, follow);
      window.clearTimeout(settleTimer);
      forgetGesture(root);
    };
  }, [backMovesPage]);
}

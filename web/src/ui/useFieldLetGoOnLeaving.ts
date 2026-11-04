import { useEffect } from "react";
import { hasOnScreenKeyboard, isTextEntry, letGoOfFieldWhileKeyboardClosed } from "./softKeyboard";

// useFieldLetGoOnLeaving keeps the phone's keyboard closed when the app comes back. Android brings it up again for the
// field that still has the focus, even when the person had closed it before leaving, and half of what they came back
// to see is gone. A keyboard that was open stays open: they were typing and can go on. What they were still composing
// (kana not yet converted) is committed first: on return, the IME's next key replaces it (seen with Gboard).
export function useFieldLetGoOnLeaving(): void {
  useEffect(() => {
    let composing = false;
    const compositionStarted = () => { composing = true; };
    const compositionEnded = () => { composing = false; };
    function letGoOnLeaving() {
      if (!document.hidden || !hasOnScreenKeyboard()) return;
      const active = document.activeElement;
      if (composing && isTextEntry(active)) {
        active.blur();
        active.focus();
      }
      letGoOfFieldWhileKeyboardClosed();
    }
    document.addEventListener("compositionstart", compositionStarted);
    document.addEventListener("compositionend", compositionEnded);
    document.addEventListener("visibilitychange", letGoOnLeaving);
    return () => {
      document.removeEventListener("compositionstart", compositionStarted);
      document.removeEventListener("compositionend", compositionEnded);
      document.removeEventListener("visibilitychange", letGoOnLeaving);
    };
  }, []);
}

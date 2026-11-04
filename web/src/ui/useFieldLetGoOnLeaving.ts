import { useEffect } from "react";
import { hasOnScreenKeyboard, letGoOfFieldWhileKeyboardClosed } from "./softKeyboard";

// useFieldLetGoOnLeaving keeps the phone's keyboard closed when the app comes back. Android brings it up again for the
// field that still has the focus, even when the person had closed it before leaving, and half of what they came back
// to see is gone. A keyboard that was open stays open: they were typing and can go on.
export function useFieldLetGoOnLeaving(): void {
  useEffect(() => {
    function letGoOnLeaving() {
      if (document.hidden && hasOnScreenKeyboard()) letGoOfFieldWhileKeyboardClosed();
    }
    document.addEventListener("visibilitychange", letGoOnLeaving);
    return () => document.removeEventListener("visibilitychange", letGoOnLeaving);
  }, []);
}

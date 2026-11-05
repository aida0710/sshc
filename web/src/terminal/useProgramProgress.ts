import { useCallback, useRef, useState, type SetStateAction } from "react";
import { noTerminalProgress, type TerminalProgress } from "./osc9Progress";

// useProgramProgress keeps the OSC 9;4 progress a pane's header shows, and calls changed each time a sequence changes
// it. The session's row draws the same progress from the session list, so changed can read the list again and the row
// change together with the header instead of on the next poll.
export function useProgramProgress(changed: () => void) {
  const [progress, setProgress] = useState<TerminalProgress>(noTerminalProgress);
  const shown = useRef<TerminalProgress>(noTerminalProgress);
  const changedRef = useRef(changed);
  changedRef.current = changed;

  // apply takes what a sequence said. Programs repeat the same report, and a repeat changes nothing on screen, so it
  // does not call changed.
  const apply = useCallback((update: SetStateAction<TerminalProgress>) => {
    const next = typeof update === "function" ? update(shown.current) : update;
    if (sameProgress(next, shown.current)) return;
    shown.current = next;
    setProgress(next);
    changedRef.current();
  }, []);

  // clear forgets the progress when the view starts over with another session or shell. The engine clears its own
  // when the shell is replaced (internal/terminal/session.go), so the list it gives already says so.
  const clear = useCallback(() => {
    shown.current = noTerminalProgress;
    setProgress(noTerminalProgress);
  }, []);

  return { progress, apply, clear };
}

function sameProgress(a: TerminalProgress, b: TerminalProgress): boolean {
  return a.state === b.state && percentOf(a) === percentOf(b);
}

function percentOf(progress: TerminalProgress): number | undefined {
  return "percent" in progress ? progress.percent : undefined;
}

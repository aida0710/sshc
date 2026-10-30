import { useCallback, useEffect, useRef } from "react";

// settingRestDelayMs is how long a setting moved by a drag or a slider rests before it is saved: the control
// moves it many times a second, and each save is a change of the synced settings, so only where it comes to
// rest is kept.
const settingRestDelayMs = 600;

// useRestingSave returns a function that saves its value once it has rested for settingRestDelayMs. A value
// still waiting when the component goes away is saved then, so leaving the page does not lose it.
export function useRestingSave<T>(save: (value: T) => void): (value: T) => void {
  const timer = useRef<number | undefined>(undefined);
  const waiting = useRef<{ value: T } | null>(null);
  const latestSave = useRef(save);
  latestSave.current = save;

  useEffect(() => () => {
    window.clearTimeout(timer.current);
    if (waiting.current !== null) latestSave.current(waiting.current.value);
  }, []);

  return useCallback((value: T) => {
    waiting.current = { value };
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      waiting.current = null;
      latestSave.current(value);
    }, settingRestDelayMs);
  }, []);
}

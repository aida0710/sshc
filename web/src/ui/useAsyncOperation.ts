import { useCallback, useMemo, useState } from "react";

// The three facts a screen reports about an operation it runs against the
// engine: whether one is in flight, what the last one said went wrong, and
// whether the last one landed. Every form, panel and dialog that talks to the
// engine used to keep its own trio of useState; this is that trio, once.
//
// The returned object is stable between renders while its facts are unchanged,
// so it can sit in an effect's dependency list without re-running the effect.
export type AsyncOperation = {
  busy: boolean;
  error: string;
  saved: boolean;
  // Editing clears the "saved" mark so that it never describes a stale form.
  touch: () => void;
  fail: (message: string) => void;
  clearError: () => void;
  // Runs work, reporting busy while it runs and either "saved" or the
  // described failure afterwards. Resolves to whether the work succeeded.
  run: <T>(work: () => Promise<T>, options?: RunOptions<T>) => Promise<boolean>;
};

export type RunOptions<T> = {
  // Receives the successful result, for callers that keep state from it.
  apply?: (value: T) => void;
  // Turns the failure into the message to show. Without it the operation
  // reports the error's message, or nothing.
  describe?: (error: unknown) => string;
};

export function useAsyncOperation(): AsyncOperation {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  const touch = useCallback(() => setSaved(false), []);
  const clearError = useCallback(() => setError(""), []);
  const fail = useCallback((message: string) => {
    setError(message);
    setSaved(false);
  }, []);
  const run = useCallback(async <T,>(work: () => Promise<T>, options: RunOptions<T> = {}): Promise<boolean> => {
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      const value = await work();
      options.apply?.(value);
      setSaved(true);
      return true;
    } catch (caught) {
      setError(options.describe ? options.describe(caught) : caught instanceof Error ? caught.message : "");
      return false;
    } finally {
      setBusy(false);
    }
  }, []);

  return useMemo(
    () => ({ busy, error, saved, touch, fail, clearError, run }),
    [busy, error, saved, touch, fail, clearError, run],
  );
}

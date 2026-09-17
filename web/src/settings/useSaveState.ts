import { useState } from "react";

// The three facts every settings form reports: whether a save is running,
// what went wrong, and whether the last save landed. Editing clears the
// "saved" mark so that it never describes a stale form.
export function useSaveState() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  return {
    busy,
    error,
    saved,
    touch: () => setSaved(false),
    fail: (message: string) => {
      setError(message);
      setSaved(false);
    },
    async run(work: () => Promise<void>, describe: (error: unknown) => string): Promise<boolean> {
      setBusy(true);
      setError("");
      setSaved(false);
      try {
        await work();
        setSaved(true);
        return true;
      } catch (error) {
        setError(describe(error));
        return false;
      } finally {
        setBusy(false);
      }
    },
  };
}

export type SaveState = ReturnType<typeof useSaveState>;

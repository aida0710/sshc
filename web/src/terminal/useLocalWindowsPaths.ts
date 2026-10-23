import { useEffect, useState } from "react";
import { aboutApi } from "../api/about";
import type { TerminalSession } from "../api/terminalSessions";

// Browser OS and terminal output do not identify the filesystem we preview.
// Only this engine's local reader is known to accept native Windows paths;
// a run target is previewed through SSH/SFTP instead.
export function useLocalWindowsPaths(kind: TerminalSession["kind"], runTarget: string | undefined): boolean {
  const [windows, setWindows] = useState(false);
  const localShell = kind === "shell" && runTarget === undefined;
  useEffect(() => {
    if (!localShell) return;
    let cancelled = false;
    void aboutApi.about().then((about) => {
      if (!cancelled) setWindows(about.os === "windows");
    }).catch(() => { if (!cancelled) setWindows(false); });
    return () => { cancelled = true; };
  }, [localShell]);
  return localShell && windows;
}

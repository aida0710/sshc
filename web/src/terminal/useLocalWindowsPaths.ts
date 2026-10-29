import { useEngineOS } from "../about/useEngineOS";
import type { TerminalSession } from "../api/terminalSessions";

// Browser OS and terminal output do not identify the filesystem we preview.
// Only this engine's local reader is known to accept native Windows paths;
// a run target is previewed through SSH/SFTP instead.
export function useLocalWindowsPaths(kind: TerminalSession["kind"], runTarget: string | undefined): boolean {
  const localShell = kind === "shell" && runTarget === undefined;
  const os = useEngineOS(localShell);
  return localShell && os === "windows";
}

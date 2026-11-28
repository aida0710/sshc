import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import { useThisMachineSettings } from "../settings/settingsStore";
import { offScreenProgramStatus } from "./offScreenProgramStatus";
import type { MarkedProgramState, SeenProgramStatus } from "./programStatus";
import { groupSessionsByWorkspace, showsHiddenWorkspaces } from "./workspaceGrouping";

// useNavigationAttention は、ナビゲーションを開くボタンの角に出す印を求める（offScreenProgramStatus）。数えるのは
// サイドバーの木が並べるセッションだけにする。隠したワークスペース（既定では agents）のセッションには行が無いので、
// 数えると、ナビゲーションを開いても印の理由が見つからず、印も消せない。
export function useNavigationAttention({ sessions, workspaces, seen, sessionsOnScreen }: {
  sessions: readonly TerminalSession[];
  workspaces: readonly TerminalWorkspace[];
  seen: SeenProgramStatus;
  // sessionsOnScreen は、画面に出ているセッションの id である。ターミナルの画面でなければ空。
  sessionsOnScreen: readonly string[];
}): MarkedProgramState | null {
  const showHidden = showsHiddenWorkspaces(useThisMachineSettings().document);
  const sessionsInTree = groupSessionsByWorkspace(workspaces, sessions, showHidden).flatMap((group) => group.sessions);
  return offScreenProgramStatus({ sessions: sessionsInTree, seen, sessionsOnScreen });
}

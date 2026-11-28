import type { TerminalSession } from "../api/terminalSessions";
import { rowProgramStatus, type MarkedProgramState, type SeenProgramStatus } from "./programStatus";

// 急ぐ順。人を待つプログラムは、答えるまで止まったままなので先に出す。失敗は、終わったことより先に知らせる。
const statesByUrgency: readonly MarkedProgramState[] = ["blocked", "error", "done"];

// offScreenProgramStatus は、画面に出ていないセッションの行が出している印のうち、いちばん急ぐものを返す。
// 無ければ null。行と同じ rowProgramStatus で判じるので、ナビゲーションを開けば同じ印の行がある。
// 人を待つ状態は、見たことがあっても続くあいだ出し、失敗と完了は、そのセッションを画面に出すまで出す。
export function offScreenProgramStatus({ sessions, seen, sessionsOnScreen }: {
  sessions: readonly TerminalSession[];
  seen: SeenProgramStatus;
  // sessionsOnScreen は、画面に出ているセッションの id である。ターミナルの画面でなければ空。
  sessionsOnScreen: readonly string[];
}): MarkedProgramState | null {
  const shownStates = new Set(sessions
    .filter((session) => !sessionsOnScreen.includes(session.id))
    .map((session) => rowProgramStatus(session, seen)?.state));
  return statesByUrgency.find((state) => shownStates.has(state)) ?? null;
}

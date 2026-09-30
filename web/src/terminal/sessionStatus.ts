import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { connectionProgressText } from "./progress";
import { terminalProblemKey } from "./sessions";

// sessionStatusText は、ターミナルの一覧の行に出す状態の文言である。失敗の理由が
// あればそれを、無ければ再接続・接続中・接続済み・終了のどれかを出す。
export function sessionStatusText(t: Translate, session: TerminalSession): string {
  if (session.problem !== "") return t(terminalProblemKey(session.problem));
  switch (session.state) {
    case "reconnecting":
      return t("terminal.reconnectingAttempt", {
        attempt: String(session.reconnect?.attempt ?? 1),
        limit: String(session.reconnect?.limit ?? 1),
      });
    case "connecting":
      return connectionProgressText(t, session);
    case "exited":
      return t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
    default:
      return t("terminal.connected");
  }
}

// sessionMarkerClass は、行の先頭の点の色である。接続を待っているあいだは注意の色にする。
export function sessionMarkerClass(session: TerminalSession): string {
  if (session.state === "reconnecting" || session.state === "connecting") return "bg-notice-ink";
  return session.state === "exited" ? "bg-ink-faint" : "bg-live";
}

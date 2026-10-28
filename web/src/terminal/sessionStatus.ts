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
      if (session.exited?.connectionLost === true) return t("terminal.exitedConnectionLost");
      if (session.exited !== undefined && session.exited.signal !== "") return t("terminal.exitedBySignal", { signal: session.exited.signal });
      return t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
    default:
      return t("terminal.connected");
  }
}

// exitedAbnormally は、セッションが失敗で終わったか（終了コードが0以外、シグナル、接続が
// 切れた）である。行の点と状態の文、ターミナルの終了の帯を赤にする。
export function exitedAbnormally(session: TerminalSession): boolean {
  const exited = session.exited;
  if (session.state !== "exited" || exited === undefined) return false;
  return exited.code !== 0 || exited.signal !== "" || exited.connectionLost === true;
}

// sessionMarkerClass は、行の先頭の点の形と色である。未読の通知は注意の色で塗り、接続を
// 待っているあいだは注意の色の輪、失敗で終わったものは赤、ほかに終わったものは薄い色、
// つながっているものは緑にする。未読の通知に、ユーザーが選んださし色は使わない。
export function sessionMarkerClass(session: TerminalSession, unread: boolean): string {
  if (unread) return "bg-notice-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-notice-ink)_22%,transparent)]";
  if (waitsForConnection(session)) return "border-2 border-notice-ink";
  if (session.state !== "exited") return "bg-live";
  return exitedAbnormally(session) ? "bg-danger" : "bg-ink-faint";
}

// sessionShowsDestination は、行の2行目に接続先を出すかである。つながっていて問題の無いときだけ
// 出し、それ以外はその場所に状態の文（sessionStatusText）を出す。
export function sessionShowsDestination(session: TerminalSession): boolean {
  return session.problem === "" && !waitsForConnection(session) && session.state !== "exited";
}

// sessionStatusTone は、行の2行目に出す状態の文の色である。終わっただけなら薄く、失敗で
// 終わったなら赤、待っているか問題があるなら注意の色にする。
export function sessionStatusTone(session: TerminalSession): string {
  if (session.problem !== "" || session.state !== "exited") return "text-notice-ink";
  return exitedAbnormally(session) ? "text-danger" : "text-ink-faint";
}

function waitsForConnection(session: TerminalSession): boolean {
  return session.state === "reconnecting" || session.state === "connecting";
}

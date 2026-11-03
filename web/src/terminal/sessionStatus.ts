import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { connectionProgressText } from "./progress";
import { terminalProblemKey } from "./sessions";

// exitCodeUnknown is the engine's code for an exit whose status it could not learn (terminal.ExitCodeUnknown).
// It is not a status the program returned, so it is never shown as one.
const exitCodeUnknown = -1;

// sessionStatusText は、ターミナルの一覧の行に出す状態の文言である。失敗の理由が
// あればそれを、無ければ再接続・接続中・接続済み・終了のどれかを出す。
export function sessionStatusText(t: Translate, session: TerminalSession): string {
  if (session.problem !== "") return t(terminalProblemKey(session.problem));
  return sessionStateText(t, session);
}

// sessionStateText は、状態だけを短く言う（再接続中（2/5）、接続が切れました、終了コード1 など）。
// 失敗の理由は言わない。ペインの見出しと、行の2行目（理由の無いとき）に出す。
export function sessionStateText(t: Translate, session: TerminalSession): string {
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
      if (session.exited?.code === 0 && endedRightAway(session)) return t("terminal.exitedRightAway");
      if (session.exited?.code === exitCodeUnknown) return t("terminal.exitedUnknown");
      return t("terminal.exitedWith", { code: String(session.exited?.code ?? 0) });
    default:
      return t("terminal.connected");
  }
}

// rowProblemKeys は、失敗の理由を行の2行目に出す短い言い方である。全文はツールチップと、セッションを開いた
// ときの枠に出す。一度つながってから切れたものと、開けなかったものを言い分ける。
const rowProblemKeys: Readonly<Record<string, MessageKey>> = {
  reconnect_failed: "terminal.rowProblem.reconnectGaveUp",
  reconnect_exhausted: "terminal.rowProblem.reconnectGaveUp",
  reconnect_stopped: "terminal.rowProblem.reconnectStopped",
  connect_refused: "terminal.rowProblem.refused",
  connect_timed_out: "terminal.rowProblem.timedOut",
  connect_host_not_found: "terminal.rowProblem.hostNotFound",
  connect_no_network: "terminal.rowProblem.noNetwork",
  connect_host_unreachable: "terminal.rowProblem.hostUnreachable",
  connect_closed: "terminal.rowProblem.closed",
  host_key_changed: "terminal.rowProblem.hostKeyChanged",
  host_key_unknown: "terminal.rowProblem.hostKey",
  host_key_declined: "terminal.rowProblem.hostKey",
  host_key_revoked: "terminal.rowProblem.hostKey",
  known_hosts_symlink: "terminal.rowProblem.hostKey",
  authentication_rejected: "terminal.rowProblem.authentication",
  authentication_unavailable: "terminal.rowProblem.authentication",
  authentication_cancelled: "terminal.rowProblem.authentication",
  identity_unavailable: "terminal.rowProblem.authentication",
  key_passphrase_required: "terminal.rowProblem.authentication",
  proxy_authentication_required: "terminal.rowProblem.authentication",
};

// rowProblemText は、セッションの行の2行目に出す、失敗の理由の短い言い方である。
export function rowProblemText(t: Translate, problem: string): string {
  return t(rowProblemKeys[problem] ?? "terminal.rowProblem");
}

// reconnectsOnEnter は、接続の問題で終わったセッションかである。エンジンが「Enterで再接続します」と書いたので、
// 次の Enter でつなぎ直し、ほかのキーは捨てる。シェルが自分で終わったセッションは含まない。
export function reconnectsOnEnter(session: TerminalSession): boolean {
  if (session.state !== "exited" || session.kind !== "ssh" || session.alias === undefined) return false;
  return session.problem !== "" || session.exited?.connectionLost === true;
}

// endedRightAwayMs より早く終わったセッションは、終了コードが0でも、シェルやその
// プロファイルの設定が誤っていて動かなかったとみなす。Ghostty の
// abnormal-command-exit-runtime の既定と同じ長さである。
const endedRightAwayMs = 250;

// endedRightAway は、セッションが開いてすぐ（endedRightAwayMs 未満で）終わったかである。
// 終わった時刻は一覧が届くまで分からないので、それまでは false になる。
export function endedRightAway(session: TerminalSession): boolean {
  const exited = session.exited;
  if (session.state !== "exited" || exited === undefined) return false;
  const ranMs = Date.parse(exited.at) - Date.parse(session.startedAt);
  return Number.isFinite(ranMs) && ranMs < endedRightAwayMs;
}

// exitedAbnormally は、セッションが失敗で終わったか（終了コードが0以外、シグナル、接続が
// 切れた、開いてすぐ終わった）である。行の点と状態の文、ターミナルの終了の帯を赤にする。
export function exitedAbnormally(session: TerminalSession): boolean {
  const exited = session.exited;
  if (session.state !== "exited" || exited === undefined) return false;
  return exited.code !== 0 || exited.signal !== "" || exited.connectionLost === true || endedRightAway(session);
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

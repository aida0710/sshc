import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { exitText } from "./exitText";
import { terminalProblemKey } from "./sessions";
import { exitedAbnormally } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import { useReconnectCountdown } from "./useReconnectCountdown";
import type { SessionRecovery } from "./useSessionRecovery";
import { Button } from "../ui/surface";

// 帯は、ターミナルの上に1行ずつ重なる。共通の Notice のように操作を文の下に置くと、帯ごとに1行増えて
// ターミナルの行が減るので、操作は文の右に並べ、ボタンは知らせの中に置く低いボタン（Button small）にする。
const notice = "shrink-0 border-b border-notice-line bg-notice px-3 py-1.5 text-xs text-notice-ink";
// スマホの組み方では、指で押せるように帯のボタンを少し高くする。
const bannerButton = "shrink-0 mobile-layout:min-h-8";

function StreamLinkMessage({ link }: { link: Exclude<StreamLink, { phase: "live" }> }) {
  const t = useTranslate();
  if (link.phase === "connecting") {
    return link.attempt === 1 ? t("terminal.linkConnecting") : t("terminal.linkRetrying", { attempt: String(link.attempt) });
  }
  if (link.phase === "stopped") return link.gone ? t("terminal.linkGone") : t("terminal.linkStopped");
  // 秒読みは毎秒変わるので読み上げず、試行の回数が変わったときだけ読み上げる。
  return (
    <>
      <span aria-hidden="true">
        {t("terminal.linkWaiting", { seconds: String(link.seconds), attempt: String(link.attempt) })}
      </span>
      <span className="sr-only">{t("terminal.linkWaitingAnnouncement", { attempt: String(link.attempt) })}</span>
    </>
  );
}

// ReconnectMessage says which attempt the engine is on and whether it is waiting for the next one (with the seconds
// left) or connecting now.
function ReconnectMessage({ reconnect }: { reconnect: TerminalSession["reconnect"] }) {
  const t = useTranslate();
  const { attempt, limit, seconds } = useReconnectCountdown(reconnect);
  if (seconds === null) return t("terminal.reconnectingNow", { attempt, limit });
  // 秒読みは毎秒変わるので読み上げず、試行の回数が変わったときだけ読み上げる。
  return (
    <>
      <span aria-hidden="true">{t("terminal.reconnectingIn", { attempt, limit, seconds: String(seconds) })}</span>
      <span className="sr-only">{t("terminal.reconnectingAttempt", { attempt, limit })}</span>
    </>
  );
}

// The strip of one-line notices between the terminal header and the screen:
// the engine's reconnect progress, the browser's own stream link, and how
// the program ended. Each row only appears while it has something to say, and
// one thing is said once: an ended session's reason (why reconnecting stopped,
// why it could not connect) is its ended row, not a row of its own above it.
export function TerminalStatusBanners({ session, problem, link, onLinkNow, onLinkStop, recovery }: {
  session: TerminalSession;
  // A message from the view itself, such as a refused clipboard write.
  problem: string;
  link: StreamLink;
  onLinkNow: () => void;
  onLinkStop: () => void;
  // recovery reconnects an ended SSH session and stops the automatic reconnect; without it neither is offered.
  recovery?: SessionRecovery;
}) {
  const t = useTranslate();

  return (
    <>
      {problem === "" ? null : (
        <p role="status" className={notice}>
          {problem}
        </p>
      )}
      {recovery?.reconnectFailed === true ? (
        <p role="status" className={notice}>
          {t("terminal.manualReconnectFailed")}
        </p>
      ) : null}

      {session.state !== "reconnecting" ? null : (
        <div role="status" className={`flex flex-wrap items-center gap-2 ${notice}`}>
          <p className="min-w-0 grow">
            <ReconnectMessage reconnect={session.reconnect} />
          </p>
          {recovery?.stopReconnecting === undefined ? null : (
            <Button small disabled={recovery.stopBusy} onClick={recovery.stopReconnecting} className={bannerButton}>
              {t("terminal.stopReconnect")}
            </Button>
          )}
        </div>
      )}

      {session.problem === "" || session.exited !== undefined ? null : (
        <p role="alert" className={notice}>
          {t(terminalProblemKey(session.problem))}
        </p>
      )}

      {link.phase === "live" ? null : (
        <div role="status" className={`flex items-center gap-2 ${notice}`}>
          <p className="min-w-0 grow">
            <StreamLinkMessage link={link} />
          </p>
          {link.phase === "stopped" && link.gone ? null : (
            <Button small disabled={link.phase === "connecting"} onClick={onLinkNow} className={bannerButton}>
              {t("terminal.linkNow")}
            </Button>
          )}
          {link.phase === "stopped" ? null : (
            <Button small onClick={onLinkStop} className={bannerButton}>
              {t("terminal.linkStop")}
            </Button>
          )}
        </div>
      )}
      {session.exited === undefined ? null : (
        <div role={session.problem === "" ? "status" : "alert"} className="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-card px-3 py-1.5 text-xs">
          <p className={`min-w-0 grow ${session.problem !== "" || exitedAbnormally(session) ? "text-danger" : "text-ink-muted"}`}>
            {session.problem === "" ? exitText(t, session.exited, session) : t(terminalProblemKey(session.problem))}
          </p>
          {session.kind !== "ssh" || session.alias === undefined || recovery?.reconnect === undefined ? null : (
            <Button small disabled={recovery.reconnectBusy} onClick={recovery.reconnect} className={bannerButton}>
              {t(recovery.reconnectBusy ? "terminal.manualReconnecting" : "terminal.manualReconnect")}
            </Button>
          )}
        </div>
      )}
    </>
  );
}

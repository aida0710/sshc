import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { exitText } from "./exitText";
import { terminalProblemKey } from "./sessions";
import { exitedAbnormally, reconnectAwaitsInput } from "./sessionStatus";
import type { StreamLink } from "./streamLink";
import { hostKeyQuestionOf, statusBannerRows, type StatusBannerRow } from "./statusBannerRows";
import { useReconnectCountdown } from "./useReconnectCountdown";
import type { HostKeyAnswer } from "../shell/useHostKeyAnswer";
import type { SessionRecovery } from "./useSessionRecovery";
import { Banner, bannerButton } from "../ui/Banner";
import { Button } from "../ui/surface";

// 帯は、ターミナルの上に1行ずつ重なる。共通の Notice のように操作を文の下に置くと、帯ごとに1行増えて
// ターミナルの行が減るので、区画の上の帯（Banner）にして、操作は文の右に並べる。

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
function ReconnectMessage({ reconnect, awaitingInput }: { reconnect: TerminalSession["reconnect"]; awaitingInput: boolean }) {
  const t = useTranslate();
  const { attempt, limit, seconds } = useReconnectCountdown(reconnect);
  if (reconnect?.waitingForNetwork === true) return t("terminal.waitingForNetworkDetail");
  if (awaitingInput) return t("terminal.reconnectingAwaitingInputDetail", { attempt, limit });
  if (seconds === null) return t("terminal.reconnectingNow", { attempt, limit });
  // 秒読みは毎秒変わるので読み上げず、試行の回数が変わったときだけ読み上げる。
  return (
    <>
      <span aria-hidden="true">{t("terminal.reconnectingIn", { attempt, limit, seconds: String(seconds) })}</span>
      <span className="sr-only">{t("terminal.reconnectingAttempt", { attempt, limit })}</span>
    </>
  );
}

// The strip of one-line notices between the terminal header and the screen: the engine's reconnect progress, the
// browser's own stream link, and how the program ended. Which rows it shows is statusBannerRows.
export function TerminalStatusBanners({ session, problem, link, onLinkNow, onLinkStop, recovery, hostKeyAnswer, onResumeProgram, hiddenRows = [] }: {
  session: TerminalSession;
  // A message from the view itself, such as a refused clipboard write.
  problem: string;
  link: StreamLink;
  onLinkNow: () => void;
  onLinkStop: () => void;
  // recovery reconnects an ended SSH session and stops the automatic reconnect; without it neither is offered.
  recovery?: SessionRecovery;
  // hostKeyAnswer answers the question about an unknown host key; without it the terminal is the way to answer.
  hostKeyAnswer?: HostKeyAnswer;
  // onResumeProgram types the command that starts the previous shell's program again.
  onResumeProgram?: (command: string) => void;
  // hiddenRows are the rows left out because something else on the screen says them now.
  hiddenRows?: readonly StatusBannerRow[];
}) {
  const t = useTranslate();
  const question = hostKeyQuestionOf(session);
  const rows = new Set(statusBannerRows({
    session, problem, link, reconnectFailed: recovery?.reconnectFailed === true, answersHostKey: hostKeyAnswer?.answer !== undefined,
  }).filter((row) => !hiddenRows.includes(row)));

  return (
    <>
      {!rows.has("view-problem") ? null : (
        <Banner tone="notice" role="status">{problem}</Banner>
      )}
      {rows.has("reconnect-failed") ? (
        <Banner tone="notice" role="status">{t("terminal.manualReconnectFailed")}</Banner>
      ) : null}

      {!rows.has("host-key") || question === undefined || hostKeyAnswer === undefined ? null : (
        <Banner
          tone="notice"
          role="status"
          actions={(
            <>
              {/* busy-label-exempt: ［やめる］は「〇〇中」の形にならないので、文言を替えず回る印だけを添える（2026-11-25 利用者が決めた）。 */}
              <Button small busy={hostKeyAnswer.answering === false} busyLabel={null} disabled={hostKeyAnswer.answering !== null} onClick={() => hostKeyAnswer.answer?.(question.fingerprint, false)} className={bannerButton}>
                {t("terminal.hostKeyReject")}
              </Button>
              <Button small busy={hostKeyAnswer.answering === true} busyLabel={t("busy.connecting")} disabled={hostKeyAnswer.answering !== null} onClick={() => hostKeyAnswer.answer?.(question.fingerprint, true)} className={bannerButton}>
                {t("terminal.hostKeyAccept")}
              </Button>
            </>
          )}
        >
          {t("terminal.noticeCheckHostKey", { target: question.alias || question.hostName })}
        </Banner>
      )}

      {!rows.has("reconnecting") ? null : (
        <Banner
          tone="notice"
          role="status"
          actions={recovery?.stopReconnecting === undefined ? undefined : (
            <Button small busy={recovery.stopBusy} busyLabel={t("busy.stopping")} onClick={recovery.stopReconnecting} className={bannerButton}>
              {t("terminal.stopReconnect")}
            </Button>
          )}
        >
          <ReconnectMessage reconnect={session.reconnect} awaitingInput={reconnectAwaitsInput(session)} />
        </Banner>
      )}

      {!rows.has("connection-problem") ? null : (
        <Banner tone="notice" role="alert">{t(terminalProblemKey(session.problem))}</Banner>
      )}

      {!rows.has("link") || link.phase === "live" ? null : (
        <Banner
          tone="notice"
          role="status"
          actions={link.phase === "stopped" && link.gone ? undefined : (
            <>
              <Button small disabled={link.phase === "connecting"} onClick={onLinkNow} className={bannerButton}>
                {t("terminal.linkNow")}
              </Button>
              {link.phase === "stopped" ? null : (
                <Button small onClick={onLinkStop} className={bannerButton}>
                  {t("terminal.linkStop")}
                </Button>
              )}
            </>
          )}
        >
          <StreamLinkMessage link={link} />
        </Banner>
      )}
      {!rows.has("resume-program") || session.resume === undefined ? null : (
        <Banner
          tone="notice"
          role="status"
          actions={onResumeProgram === undefined ? undefined : (
            <Button small onClick={() => onResumeProgram(session.resume?.command ?? "")} className={bannerButton}>
              {t("terminal.resumeRun")}
            </Button>
          )}
        >
          {t("terminal.resumeTitle", { program: session.resume.program })}
        </Banner>
      )}
      {!rows.has("ended") || session.exited === undefined ? null : (
        <Banner
          tone="quiet"
          role={session.problem === "" ? "status" : "alert"}
          actions={session.kind !== "ssh" || session.alias === undefined || recovery?.reconnect === undefined ? undefined : (
            <Button small busy={recovery.reconnectBusy} busyLabel={t("busy.connecting")} onClick={recovery.reconnect} className={bannerButton}>
              {t("terminal.manualReconnect")}
            </Button>
          )}
        >
          <span className={session.problem !== "" || exitedAbnormally(session) ? "text-danger" : "text-ink-muted"}>
            {session.problem === "" ? exitText(t, session.exited, session) : t(terminalProblemKey(session.problem))}
          </span>
        </Banner>
      )}
    </>
  );
}

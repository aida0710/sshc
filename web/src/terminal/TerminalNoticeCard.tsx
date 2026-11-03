import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { HostKeyQuestionCard } from "../shell/HostKeyQuestionCard";
import type { HostKeyAnswer } from "../shell/useHostKeyAnswer";
import { NoticeFrame, noticeCardButton, type NoticeFocus } from "../ui/NoticeFrame";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { exitText } from "./exitText";
import { terminalProblemKey } from "./sessions";
import type { TerminalNotice } from "./terminalNotice";
import { useReconnectCountdown } from "./useReconnectCountdown";
import type { SessionRecovery } from "./useSessionRecovery";

// TerminalNoticeCard shows a state of the session that waits on the person, larger than the strip above the
// terminal and with the same actions. The strip still says it: the card is not a live region, so a screen reader
// hears each change once, and the person can hide the card to read the log under it and act from the strip.
export function TerminalNoticeCard({ notice, session, title, recovery, hostKeyAnswer, focus, onLinkNow, onCloseSession, onDismiss }: {
  notice: TerminalNotice;
  session: TerminalSession;
  // title is the session's name as the header shows it.
  title: string;
  recovery: SessionRecovery;
  hostKeyAnswer: HostKeyAnswer;
  focus: NoticeFocus;
  onLinkNow: () => void;
  onCloseSession: (() => void) | undefined;
  onDismiss: () => void;
}) {
  const frame = { focus, onDismiss };
  switch (notice.kind) {
    case "host-key":
      return session.hostKeyQuestion === undefined ? null : (
        <HostKeyQuestionCard question={session.hostKeyQuestion} hostKeyAnswer={hostKeyAnswer} focus={focus} onDismiss={onDismiss} />
      );
    case "reconnecting":
      return <ReconnectingCard session={session} title={title} recovery={recovery} frame={frame} />;
    case "ended":
      return <EndedCard session={session} recovery={recovery} onCloseSession={onCloseSession} frame={frame} />;
    case "link-stopped":
      return <LinkStoppedCard onLinkNow={onLinkNow} frame={frame} />;
  }
}

// FrameBehaviour is what every card does the same way: take the focus, and hide (NoticeFrame).
type FrameBehaviour = { focus: NoticeFocus; onDismiss: () => void };

function ReconnectingCard({ session, title, recovery, frame }: {
  session: TerminalSession;
  title: string;
  recovery: SessionRecovery;
  frame: FrameBehaviour;
}) {
  const t = useTranslate();
  const { attempt, limit, seconds } = useReconnectCountdown(session.reconnect);
  return (
    <NoticeFrame
      tone="notice"
      kicker={<><Spinner className="size-3.5" />{t("terminal.noticeReconnecting", { attempt, limit })}</>}
      title={t("terminal.noticeConnectionLost", { title })}
      body={seconds === null ? t("terminal.noticeTryingNow") : t("terminal.noticeNextTry", { seconds: String(seconds) })}
      {...frame}
    >
      {recovery.stopReconnecting === undefined ? null : (
        <Button busy={recovery.stopBusy} onClick={recovery.stopReconnecting} className={noticeCardButton}>{t("terminal.stopReconnect")}</Button>
      )}
    </NoticeFrame>
  );
}

function EndedCard({ session, recovery, onCloseSession, frame }: {
  session: TerminalSession;
  recovery: SessionRecovery;
  onCloseSession: (() => void) | undefined;
  frame: FrameBehaviour;
}) {
  const t = useTranslate();
  const reason = session.problem !== ""
    ? t(terminalProblemKey(session.problem))
    : session.exited === undefined ? "" : exitText(t, session.exited, session);
  const reconnects = session.kind === "ssh" && session.alias !== undefined && recovery.reconnect !== undefined;
  return (
    <NoticeFrame
      tone="ended"
      title={reason}
      body={recovery.reconnectFailed ? t("terminal.manualReconnectFailed") : undefined}
      {...frame}
    >
      {onCloseSession === undefined ? null : <Button onClick={onCloseSession} className={noticeCardButton}>{t("terminal.noticeCloseSession")}</Button>}
      {reconnects ? (
        <Button kind="primary" busy={recovery.reconnectBusy} onClick={recovery.reconnect} className={noticeCardButton}>{t("terminal.manualReconnect")}</Button>
      ) : null}
    </NoticeFrame>
  );
}

function LinkStoppedCard({ onLinkNow, frame }: { onLinkNow: () => void; frame: FrameBehaviour }) {
  const t = useTranslate();
  return (
    <NoticeFrame tone="notice" title={t("terminal.noticeLinkLost")} body={t("terminal.linkStopped")} {...frame}>
      <Button kind="primary" onClick={onLinkNow} className={noticeCardButton}>{t("terminal.linkNow")}</Button>
    </NoticeFrame>
  );
}

import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from "react";
import type { TerminalHostKeyQuestion, TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { exitText } from "./exitText";
import { hostKeyTypeName } from "./hostKeyTypeName";
import { terminalProblemKey } from "./sessions";
import type { TerminalNotice } from "./terminalNotice";
import { useReconnectCountdown } from "./useReconnectCountdown";
import type { HostKeyAnswer } from "./useHostKeyAnswer";
import type { SessionRecovery } from "./useSessionRecovery";

// The colour of a card says what it waits for: a decision of the person (question), the engine working on it
// (notice), or the person after it stopped (ended).
type NoticeTone = "question" | "notice" | "ended";

const toneClasses: Record<NoticeTone, { frame: string; kicker: string }> = {
  question: { frame: "border-accent/50 bg-[color-mix(in_srgb,var(--ui-accent)_10%,var(--ui-card))]", kicker: "text-accent" },
  notice: { frame: "border-notice-line bg-[color-mix(in_srgb,var(--ui-notice-ink)_8%,var(--ui-card))]", kicker: "text-notice-ink" },
  ended: { frame: "border-danger/45 bg-[color-mix(in_srgb,var(--ui-danger)_9%,var(--ui-card))]", kicker: "text-danger" },
};

// A card's buttons are as tall as a finger needs on a phone.
const cardButton = "mobile-layout:min-h-11";

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
        <HostKeyCard question={session.hostKeyQuestion} hostKeyAnswer={hostKeyAnswer} frame={frame} />
      );
    case "reconnecting":
      return <ReconnectingCard session={session} title={title} recovery={recovery} frame={frame} />;
    case "ended":
      return <EndedCard session={session} recovery={recovery} onCloseSession={onCloseSession} frame={frame} />;
    case "link-stopped":
      return <LinkStoppedCard onLinkNow={onLinkNow} frame={frame} />;
  }
}

// NoticeFocus says whether a card takes the focus when it appears, and where the focus goes when the card goes away
// with the focus in it (hidden by the person, or the state it said is over).
export type NoticeFocus = {
  take: boolean;
  giveBack: () => void;
};

// FrameBehaviour is what every card does the same way: take the focus, and hide.
type FrameBehaviour = { focus: NoticeFocus; onDismiss: () => void };

// HostKeyCard asks whether to trust a host key known_hosts does not have, as the terminal does in words: the
// person compares the fingerprint with the one the host's administrator gave.
function HostKeyCard({ question, hostKeyAnswer, frame }: {
  question: TerminalHostKeyQuestion;
  hostKeyAnswer: HostKeyAnswer;
  frame: FrameBehaviour;
}) {
  const t = useTranslate();
  const { answer, busy } = hostKeyAnswer;
  return (
    <NoticeFrame
      tone="question"
      kicker={t("terminal.noticeFirstConnection")}
      title={t("terminal.noticeCheckHostKey", { target: question.alias || question.hostName })}
      detail={<p className="mt-1.5 break-all font-mono text-xs leading-5 text-ink-muted">{hostKeyTypeName(question.keyType)} {question.fingerprint}</p>}
      frame={frame}
    >
      {answer === undefined ? null : (
        <>
          <Button disabled={busy} onClick={() => answer(question.fingerprint, false)} className={cardButton}>{t("terminal.hostKeyReject")}</Button>
          <Button kind="primary" busy={busy} onClick={() => answer(question.fingerprint, true)} className={cardButton}>{t("terminal.hostKeyAccept")}</Button>
        </>
      )}
    </NoticeFrame>
  );
}

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
      frame={frame}
    >
      {recovery.stopReconnecting === undefined ? null : (
        <Button busy={recovery.stopBusy} onClick={recovery.stopReconnecting} className={cardButton}>{t("terminal.stopReconnect")}</Button>
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
      frame={frame}
    >
      {onCloseSession === undefined ? null : <Button onClick={onCloseSession} className={cardButton}>{t("terminal.noticeCloseSession")}</Button>}
      {reconnects ? (
        <Button kind="primary" busy={recovery.reconnectBusy} onClick={recovery.reconnect} className={cardButton}>{t("terminal.manualReconnect")}</Button>
      ) : null}
    </NoticeFrame>
  );
}

function LinkStoppedCard({ onLinkNow, frame }: { onLinkNow: () => void; frame: FrameBehaviour }) {
  const t = useTranslate();
  return (
    <NoticeFrame tone="notice" title={t("terminal.noticeLinkLost")} body={t("terminal.linkStopped")} frame={frame}>
      <Button kind="primary" onClick={onLinkNow} className={cardButton}>{t("terminal.linkNow")}</Button>
    </NoticeFrame>
  );
}

function NoticeFrame({ tone, kicker, title, body, detail, frame, children }: {
  tone: NoticeTone;
  // kicker is a line over the title for a state the title alone does not say (how far a reconnect is); a card that has
  // stopped says what happened in its title and the colour of its frame.
  kicker?: ReactNode;
  title: string;
  body?: string | undefined;
  // detail is shown under the body as it is, for what is not a sentence (a fingerprint).
  detail?: ReactNode;
  frame: FrameBehaviour;
  children: ReactNode;
}) {
  const t = useTranslate();
  const titleId = useId();
  const colours = toneClasses[tone];
  const section = useRef<HTMLElement>(null);
  const { focus, onDismiss } = frame;
  const giveBack = useRef(focus.giveBack);
  giveBack.current = focus.giveBack;
  // The card itself takes the focus, not one of its buttons: an Enter typed for the terminal as the card appears
  // would otherwise trust a host key or stop a reconnect. Tab reaches the buttons; a screen reader reads the title.
  // Asked only when the card appears: a card already shown does not take the focus back.
  const takesFocusOnShow = useRef(focus.take);
  useEffect(() => {
    if (takesFocusOnShow.current) section.current?.focus({ preventScroll: true });
  }, []);
  // Run while the card is still in the page (a layout effect's cleanup comes before its nodes are removed), so it
  // can tell the focus was in it.
  useLayoutEffect(() => {
    const node = section.current;
    return () => {
      if (node?.contains(document.activeElement)) giveBack.current();
    };
  }, []);
  return (
    <section
      ref={section}
      tabIndex={-1}
      aria-labelledby={titleId}
      data-terminal-notice={tone}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.stopPropagation();
        onDismiss();
      }}
      className={`pointer-events-auto relative w-full max-w-[560px] rounded-xl border p-4 pr-12 shadow-float outline-none focus-visible:ring-2 focus-visible:ring-accent ${colours.frame}`}
    >
      {kicker === undefined ? null : <p className={`mb-1 flex items-center gap-2 text-xs font-semibold ${colours.kicker}`}>{kicker}</p>}
      <h2 id={titleId} className="break-words text-sm font-semibold text-ink mobile-layout:text-[15px]">{title}</h2>
      {body === undefined ? null : <p className="mt-1 break-words text-[13px] leading-5 text-ink-muted">{body}</p>}
      {detail}
      {/* The actions sit under the words, inside the card (the way of every notice in sshc). */}
      <div className="mt-3 flex flex-wrap gap-2">{children}</div>
      <button
        type="button"
        aria-label={t("terminal.noticeDismiss")}
        title={t("terminal.noticeDismiss")}
        onClick={onDismiss}
        className="absolute right-2 top-2 flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink mobile-layout:right-0.5 mobile-layout:top-0.5 mobile-layout:size-11"
      >
        <Icon name="close" className="size-4" />
      </button>
    </section>
  );
}

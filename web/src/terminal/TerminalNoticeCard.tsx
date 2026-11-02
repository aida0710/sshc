import { useId, type ReactNode } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Spinner } from "../ui/Spinner";
import { Button } from "../ui/surface";
import { exitText } from "./exitText";
import { terminalProblemKey } from "./sessions";
import type { TerminalNotice } from "./terminalNotice";
import { useReconnectCountdown } from "./useReconnectCountdown";
import type { SessionRecovery } from "./useSessionRecovery";

// The colour of a card says what it waits for: the engine working on it (notice) or the person (ended).
type NoticeTone = "notice" | "ended";

const toneClasses: Record<NoticeTone, { frame: string; kicker: string }> = {
  notice: { frame: "border-notice-line bg-[color-mix(in_srgb,var(--ui-notice-ink)_8%,var(--ui-card))]", kicker: "text-notice-ink" },
  ended: { frame: "border-danger/45 bg-[color-mix(in_srgb,var(--ui-danger)_9%,var(--ui-card))]", kicker: "text-danger" },
};

// A card's buttons are as tall as a finger needs on a phone.
const cardButton = "mobile-layout:min-h-11";

// TerminalNoticeCard shows a state of the session that waits on the person, larger than the strip above the
// terminal and with the same actions. The strip still says it: the card is not a live region, so a screen reader
// hears each change once, and the person can hide the card to read the log under it and act from the strip.
export function TerminalNoticeCard({ notice, session, title, recovery, onLinkNow, onCloseSession, onDismiss }: {
  notice: TerminalNotice;
  session: TerminalSession;
  // title is the session's name as the header shows it.
  title: string;
  recovery: SessionRecovery;
  onLinkNow: () => void;
  onCloseSession: (() => void) | undefined;
  onDismiss: () => void;
}) {
  switch (notice.kind) {
    case "reconnecting":
      return <ReconnectingCard session={session} title={title} recovery={recovery} onDismiss={onDismiss} />;
    case "ended":
      return <EndedCard session={session} recovery={recovery} onCloseSession={onCloseSession} onDismiss={onDismiss} />;
    case "link-stopped":
      return <LinkStoppedCard onLinkNow={onLinkNow} onDismiss={onDismiss} />;
  }
}

function ReconnectingCard({ session, title, recovery, onDismiss }: {
  session: TerminalSession;
  title: string;
  recovery: SessionRecovery;
  onDismiss: () => void;
}) {
  const t = useTranslate();
  const { attempt, limit, seconds } = useReconnectCountdown(session.reconnect);
  return (
    <NoticeFrame
      tone="notice"
      kicker={<><Spinner className="size-3.5" />{t("terminal.noticeReconnecting", { attempt, limit })}</>}
      title={t("terminal.noticeConnectionLost", { title })}
      body={seconds === null ? t("terminal.noticeTryingNow") : t("terminal.noticeNextTry", { seconds: String(seconds) })}
      onDismiss={onDismiss}
    >
      {recovery.stopReconnecting === undefined ? null : (
        <Button busy={recovery.stopBusy} onClick={recovery.stopReconnecting} className={cardButton}>{t("terminal.stopReconnect")}</Button>
      )}
    </NoticeFrame>
  );
}

function EndedCard({ session, recovery, onCloseSession, onDismiss }: {
  session: TerminalSession;
  recovery: SessionRecovery;
  onCloseSession: (() => void) | undefined;
  onDismiss: () => void;
}) {
  const t = useTranslate();
  const reason = session.problem !== ""
    ? t(terminalProblemKey(session.problem))
    : session.exited === undefined ? "" : exitText(t, session.exited, session);
  const reconnects = session.kind === "ssh" && session.alias !== undefined && recovery.reconnect !== undefined;
  return (
    <NoticeFrame
      tone="ended"
      kicker={t("terminal.noticeStopped")}
      title={reason}
      body={recovery.reconnectFailed ? t("terminal.manualReconnectFailed") : undefined}
      onDismiss={onDismiss}
    >
      {onCloseSession === undefined ? null : <Button onClick={onCloseSession} className={cardButton}>{t("terminal.noticeCloseSession")}</Button>}
      {reconnects ? (
        <Button kind="primary" busy={recovery.reconnectBusy} onClick={recovery.reconnect} className={cardButton}>{t("terminal.manualReconnect")}</Button>
      ) : null}
    </NoticeFrame>
  );
}

function LinkStoppedCard({ onLinkNow, onDismiss }: { onLinkNow: () => void; onDismiss: () => void }) {
  const t = useTranslate();
  return (
    <NoticeFrame tone="notice" kicker={t("terminal.noticeStopped")} title={t("terminal.noticeLinkLost")} body={t("terminal.linkStopped")} onDismiss={onDismiss}>
      <Button kind="primary" onClick={onLinkNow} className={cardButton}>{t("terminal.linkNow")}</Button>
    </NoticeFrame>
  );
}

function NoticeFrame({ tone, kicker, title, body, onDismiss, children }: {
  tone: NoticeTone;
  kicker: ReactNode;
  title: string;
  body?: string | undefined;
  onDismiss: () => void;
  children: ReactNode;
}) {
  const t = useTranslate();
  const titleId = useId();
  const colours = toneClasses[tone];
  return (
    <section
      aria-labelledby={titleId}
      data-terminal-notice={tone}
      className={`pointer-events-auto relative w-full max-w-[560px] rounded-xl border p-4 pr-12 shadow-float ${colours.frame}`}
    >
      <p className={`flex items-center gap-2 text-xs font-semibold ${colours.kicker}`}>{kicker}</p>
      <h2 id={titleId} className="mt-1 break-words text-sm font-semibold text-ink mobile-layout:text-[15px]">{title}</h2>
      {body === undefined ? null : <p className="mt-1 break-words text-[13px] leading-5 text-ink-muted">{body}</p>}
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

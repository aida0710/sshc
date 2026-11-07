import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "./icons";

// The colour of a card says what it waits for: a decision of the person (question), the engine working on it
// (notice), or the person after it stopped (ended).
export type NoticeTone = "question" | "notice" | "ended";

const toneClasses: Record<NoticeTone, { frame: string; kicker: string }> = {
  question: { frame: "border-accent/50 bg-[color-mix(in_srgb,var(--ui-accent)_10%,var(--ui-card))]", kicker: "text-accent" },
  notice: { frame: "border-notice-line bg-[color-mix(in_srgb,var(--ui-notice-ink)_8%,var(--ui-card))]", kicker: "text-notice-ink" },
  ended: { frame: "border-danger/45 bg-[color-mix(in_srgb,var(--ui-danger)_9%,var(--ui-card))]", kicker: "text-danger" },
};

// A card's buttons are as tall as a finger needs on a phone.
export const noticeCardButton = "mobile-layout:min-h-11";

// NoticeFocus says whether a card takes the focus when it appears, and where the focus goes when the card goes away
// with the focus in it (hidden by the person, or the state it said is over).
export type NoticeFocus = {
  // take is asked once, after the card appeared: a menu or dialog that closed in the same update has gone by then, and
  // asked while drawing, it would still count as open and keep the focus from the card.
  take: () => boolean;
  giveBack: () => void;
};

// NoticeFrame is the card a notice that waits on the person is shown in, in the terminal (TerminalNoticeCard) and in an
// SFTP tab (the host key question): the words, then the actions under them inside the card (the way of every notice
// in sshc). It is not a live region; where it repeats a line read out elsewhere, that line is read once.
export function NoticeFrame({ tone, kicker, title, body, detail, focus, onDismiss, onEnter, children }: {
  tone: NoticeTone;
  // kicker is a line over the title for a state the title alone does not say (how far a reconnect is); a card that has
  // stopped says what happened in its title and the colour of its frame.
  kicker?: ReactNode;
  title: string;
  body?: string | undefined;
  // detail is shown under the body as it is, for what is not a sentence (a fingerprint).
  detail?: ReactNode;
  focus: NoticeFocus;
  // onDismiss hides the card (its × and Escape). A card with nothing under it to read has no way to hide.
  onDismiss?: (() => void) | undefined;
  // onEnter is what an Enter pressed on the card itself does, for a card whose main action is safe to take by accident
  // (connecting again). Without it an Enter does nothing, as the comment on the focus below says.
  onEnter?: (() => void) | undefined;
  children: ReactNode;
}) {
  const t = useTranslate();
  const titleId = useId();
  const colours = toneClasses[tone];
  const section = useRef<HTMLElement>(null);
  const giveBack = useRef(focus.giveBack);
  giveBack.current = focus.giveBack;
  // The card itself takes the focus, not one of its buttons: an Enter typed for the terminal as the card appears
  // would otherwise trust a host key or stop a reconnect. Tab reaches the buttons; a screen reader reads the title.
  // Asked only when the card appears: a card already shown does not take the focus back.
  const takesFocusOnShow = useRef(focus.take);
  useEffect(() => {
    if (takesFocusOnShow.current()) section.current?.focus({ preventScroll: true });
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
      data-notice-card={tone}
      onKeyDown={(event) => {
        if (event.key === "Enter" && event.target === section.current && onEnter !== undefined) {
          event.preventDefault();
          onEnter();
          return;
        }
        if (event.key !== "Escape" || onDismiss === undefined) return;
        event.stopPropagation();
        onDismiss();
      }}
      className={`pointer-events-auto relative w-full max-w-[560px] rounded-xl border p-4 shadow-float outline-none focus-visible:ring-2 focus-visible:ring-accent ${onDismiss === undefined ? "" : "pr-12"} ${colours.frame}`}
    >
      {kicker === undefined ? null : <p className={`mb-1 flex items-center gap-2 text-xs font-semibold ${colours.kicker}`}>{kicker}</p>}
      <h2 id={titleId} className="break-words text-sm font-semibold text-ink mobile-layout:text-[15px]">{title}</h2>
      {body === undefined ? null : <p className="mt-1 break-words text-[13px] leading-5 text-ink-muted">{body}</p>}
      {detail}
      <div className="mt-3 flex flex-wrap gap-2">{children}</div>
      {onDismiss === undefined ? null : (
        <button
          type="button"
          aria-label={t("terminal.noticeDismiss")}
          title={t("terminal.noticeDismiss")}
          onClick={onDismiss}
          className="absolute right-2 top-2 flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink mobile-layout:right-0.5 mobile-layout:top-0.5 mobile-layout:size-11"
        >
          <Icon name="close" className="size-4" />
        </button>
      )}
    </section>
  );
}

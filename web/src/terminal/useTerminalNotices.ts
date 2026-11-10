import { useEffect, useRef, type RefObject } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { forgetDismissedTerminalNotices, useTerminalNoticeDismissal } from "./dismissedTerminalNotices";
import { noticeSeverity, rowsSaidByCard, statusBannerRows, type NoticeSeverity, type StatusBannerRow } from "./statusBannerRows";
import type { StreamLink } from "./streamLink";
import { terminalNoticeFor, type TerminalNotice } from "./terminalNotice";
import { useMobileNotices } from "./useMobileNotices";
import { useNotificationPermissionNotice, type NotificationPermissionNotice } from "./useNotificationPermissionNotice";

// TerminalNotices は、1つのターミナルがセッションについて今出す知らせである。知らせは、ターミナルの上の帯
// （TerminalStatusBanners）と、ターミナルの下に重ねるカード（TerminalNoticeCard）の2か所に出る。
export type TerminalNotices = {
  // card はカードに出す知らせである。無ければ null。
  card: TerminalNotice | null;
  cardShown: boolean;
  dismissCard: () => void;
  // severity は、スマホの上のバーのボタンに出す重さである。知らせが1つも無ければ null。
  severity: NoticeSeverity | null;
  // stripShown は、スマホで帯をターミナルの上に重ねて出すかである。広い画面は帯をいつも見出しの下に出す。
  stripShown: boolean;
  // stripHiddenRows は、カードが同じことを言うので帯から外す行である。
  stripHiddenRows: readonly StatusBannerRow[];
  // mobileOpen は、スマホで知らせを開いているかである。
  mobileOpen: boolean;
  toggleMobile: () => void;
  notificationPermission: NotificationPermissionNotice;
  // スマホでは、この3つの外を押すと知らせを閉じる。
  buttonRef: RefObject<HTMLButtonElement | null>;
  stripRef: RefObject<HTMLDivElement | null>;
  cardFrameRef: RefObject<HTMLDivElement | null>;
};

// useTerminalNotices は、セッションとブラウザのリンクの状態から、出す知らせと出し方を決める。
export function useTerminalNotices({ session, problem, link, reconnectFailed, answersHostKey, mobile }: {
  session: TerminalSession;
  // problem は、この画面での操作が断られた理由である。無ければ空。
  problem: string;
  link: StreamLink;
  reconnectFailed: boolean;
  // answersHostKey は、この画面からホスト鍵の問いに答えられるかである。
  answersHostKey: boolean;
  mobile: boolean;
}): TerminalNotices {
  const notificationPermission = useNotificationPermissionNotice(session);
  const card = terminalNoticeFor(session, link, notificationPermission.due);
  const dismissal = useTerminalNoticeDismissal(card?.key ?? null);
  const rows = statusBannerRows({ session, problem, link, reconnectFailed, answersHostKey });
  // カードにしか出ない問い（通知の許可、打ち込んだパスワードの保存）は、帯の行に無い。
  const severity = noticeSeverity(rows, { session, link }) ?? (card === null ? null : "notice");
  const buttonRef = useRef<HTMLButtonElement>(null);
  const stripRef = useRef<HTMLDivElement>(null);
  const cardFrameRef = useRef<HTMLDivElement>(null);
  // スマホは知らせを上のバーのボタンの奥に置く。ターミナルも、その行も隠さないためである。
  const mobileNotices = useMobileNotices({
    enabled: mobile,
    present: severity !== null || card !== null,
    containerRefs: [buttonRef, stripRef, cardFrameRef],
  });
  const cardShown = card !== null && (mobile ? mobileNotices.open : !dismissal.dismissed);
  // 広い画面は、カードを隠せるあいだも帯を出しておく。スマホは両方を一度にターミナルの上へ開くので、
  // カードが言うことを帯から外し、その操作のボタンをカードの1つだけにする。
  const stripHiddenRows: readonly StatusBannerRow[] = mobile && cardShown ? rowsSaidByCard(card.kind) : [];
  const stripHasRows = rows.some((row) => !stripHiddenRows.includes(row));

  const working = session.state === "connected" && link.phase === "live";
  useEffect(() => {
    if (working) forgetDismissedTerminalNotices(session.id);
  }, [session.id, working]);

  return {
    card,
    cardShown,
    dismissCard: mobile ? mobileNotices.close : dismissal.dismiss,
    severity,
    stripShown: mobile && mobileNotices.open && stripHasRows,
    stripHiddenRows,
    mobileOpen: mobileNotices.open,
    toggleMobile: mobileNotices.toggle,
    notificationPermission,
    buttonRef,
    stripRef,
    cardFrameRef,
  };
}

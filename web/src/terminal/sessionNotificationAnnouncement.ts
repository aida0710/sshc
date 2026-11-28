import { showAndroidBlockedProgramNotification } from "../android/native";
import {
  closeBrowserNotification,
  notificationsComeFromAndroid,
  showBrowserNotification,
  type BrowserNotification,
} from "../ui/browserNotifications";
import { terminalNotificationTag } from "./notificationTags";
import { playNotificationSound } from "./notificationSound";
import type { TerminalNotification } from "./terminalNotifications";

// セッションについての通知。押すと openNotifiedSession（useOpenNotifiedSession）でそのセッションを開く。
export type SessionAnnouncement = {
  sessionId: string;
  notification: TerminalNotification;
  openNotifiedSession: (sessionId: string) => void;
};

// 同じセッションの通知は tag で置き換わり、押された Android の通知も tag からセッションを見つける。
function sessionBrowserNotification({ sessionId, notification, openNotifiedSession }: SessionAnnouncement): BrowserNotification {
  return { ...notification, tag: terminalNotificationTag(sessionId), onClick: () => openNotifiedSession(sessionId) };
}

// ターミナルのプログラムが求めた通知と、長いコマンドの終わりを知らせる。Android では音の無いチャンネルに出るので、
// どこでも音は設定の「通知音」でこの画面が鳴らす。
export function announceSessionNotification(announcement: SessionAnnouncement): void {
  playNotificationSound();
  showBrowserNotification(sessionBrowserNotification(announcement));
}

// プログラムが利用者を待ち始めたことを知らせる。Android では、このマシンのエンジンが自分のセッションに出すのと同じ
// 許可待ちのチャンネルに出す。音はそのチャンネルの設定で鳴るので、この画面が鳴らすのはブラウザだけ。Android は
// 同じ待ちを1回しか出さない（BlockedProgramWatcher.markAnnounced）。
export function announceBlockedProgram(announcement: SessionAnnouncement): void {
  const shown = sessionBrowserNotification(announcement);
  if (notificationsComeFromAndroid()) {
    showAndroidBlockedProgramNotification(shown);
    return;
  }
  playNotificationSound();
  showBrowserNotification(shown);
}

// セッションの通知を取り下げる。知らせたことを見たか、待つのをやめたか、セッションが無くなったとき。Android では、
// 取り下げた待ちをもう一度知らせられるようになる（BlockedProgramWatcher.forgetAnnounced）。
export function closeSessionNotification(sessionId: string): void {
  closeBrowserNotification(terminalNotificationTag(sessionId));
}

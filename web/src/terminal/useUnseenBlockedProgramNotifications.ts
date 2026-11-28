import { useEffect, useRef } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { notificationsComeFromAndroid } from "../ui/browserNotifications";
import type { SeenProgramStatus } from "./programStatus";
import { announceBlockedProgram } from "./sessionNotificationAnnouncement";
import { blockedProgramNotification } from "./terminalNotifications";
import { useOpenNotifiedSession } from "./useOpenNotifiedSession";

// useUnseenBlockedProgramNotifications は、ほかのセッションやページを見ている間に待ち始めたプログラムを、sshc から
// 離れるときに通知する。sshc の中で知らせるのは、サイドバーの行とナビゲーションを開くボタンの角の印
// （useNavigationAttention）だけである。通知は sshc が見えていない間の変化にしか出ない（useBlockedProgramNotifications、
// Android の BlockedProgramWatcher）ので、ここで出さないと、プログラムは待ち続ける。人が何度出入りしても1つの状態は
// 1度だけ知らせ、Android は知らせた待ちについて通知を1つに保つ。
export function useUnseenBlockedProgramNotifications(
  sessions: TerminalSession[],
  seen: SeenProgramStatus,
  { t, onOpenSession }: { t: Translate; onOpenSession: (id: string) => void },
): void {
  const openNotifiedSession = useOpenNotifiedSession(sessions, t, onOpenSession);
  const latest = useRef({ sessions, seen, t, openNotifiedSession });
  latest.current = { sessions, seen, t, openNotifiedSession };
  const announced = useRef(new Map<string, number>());

  useEffect(() => {
    function announceUnseen() {
      const { sessions: current, seen: seenNow, t: translate, openNotifiedSession: open } = latest.current;
      for (const session of current) {
        const notification = blockedProgramNotification(translate, session);
        const version = session.programStatus?.version;
        if (notification === null || version === undefined) continue;
        if ((seenNow.get(session.id) ?? 0) >= version || announced.current.get(session.id) === version) continue;
        announced.current.set(session.id, version);
        announceBlockedProgram({ sessionId: session.id, notification, openNotifiedSession: open });
      }
    }
    function announceOnHiding() {
      if (document.hidden) announceUnseen();
    }
    // A PC's window can lose the focus and stay in sight. Android's window loses it for its notification shade, which is
    // not leaving the app.
    function announceOnLosingFocus() {
      if (!document.hasFocus()) announceUnseen();
    }
    const losesFocusByLeaving = !notificationsComeFromAndroid();
    document.addEventListener("visibilitychange", announceOnHiding);
    if (losesFocusByLeaving) window.addEventListener("blur", announceOnLosingFocus);
    return () => {
      document.removeEventListener("visibilitychange", announceOnHiding);
      if (losesFocusByLeaving) window.removeEventListener("blur", announceOnLosingFocus);
    };
  }, []);
}

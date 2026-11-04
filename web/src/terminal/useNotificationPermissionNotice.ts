import { useState } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import { browserNotificationPermission } from "../ui/browserNotifications";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";

export type NotificationPermissionNotice = {
  // due is whether to ask now.
  due: boolean;
  // finish ends the notice on this device, whatever the answer was.
  finish: () => void;
};

// useNotificationPermissionNotice asks for permission to notify when it starts to matter: the first time a program says
// what it is doing (OSC 7501), since it may go on to wait for the person with sshc out of sight. The answer has to be
// remembered here: after one refusal Android still lets the app ask, and a person who closes the notice has not answered
// the browser, so the permission alone cannot tell that the person has answered.
export function useNotificationPermissionNotice(session: Pick<TerminalSession, "programStatus">): NotificationPermissionNotice {
  const [, setFinishedCount] = useState(0);
  return {
    due:
      session.programStatus !== undefined &&
      readStoredValue(localStorageKeys.notificationPermissionNoticeFinished) === null &&
      browserNotificationPermission() === "default",
    finish: () => {
      writeStoredValue(localStorageKeys.notificationPermissionNoticeFinished, "finished");
      setFinishedCount((count) => count + 1);
    },
  };
}

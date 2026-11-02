import { useState } from "react";
import { noticeKeyPrefix } from "./terminalNotice";

// The cards the person hid in this page. Kept outside the terminal view, which is unmounted when the person moves to
// another section and back; a hidden card that came back each time would be in the way of reading the log again.
const dismissed = new Set<string>();

// forgetDismissedTerminalNotices lets the cards of a session show again, once it works again: a reconnect or a link
// to the engine that is lost again later is a new occurrence of the same notice.
export function forgetDismissedTerminalNotices(sessionId: string): void {
  const prefix = noticeKeyPrefix(sessionId);
  for (const key of dismissed) {
    if (key.startsWith(prefix)) dismissed.delete(key);
  }
}

// useTerminalNoticeDismissal tells whether the card of key was hidden and hides it.
export function useTerminalNoticeDismissal(key: string | null): { dismissed: boolean; dismiss: () => void } {
  const [, setHiddenCount] = useState(0);
  return {
    dismissed: key !== null && dismissed.has(key),
    dismiss: () => {
      if (key === null) return;
      dismissed.add(key);
      setHiddenCount((count) => count + 1);
    },
  };
}

// forgetAllDismissedTerminalNotices is for tests, which share this module.
export function forgetAllDismissedTerminalNotices(): void {
  dismissed.clear();
}

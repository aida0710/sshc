// The browser's Notification API, for events that happen while sshc is out
// of sight: a terminal program asking for attention, a long command
// finishing, a background transfer ending. Browsers and native wrappers may
// lack the API or revoke delivery at any moment, so every call degrades to
// "not shown" instead of throwing.

export type BrowserNotification = {
  title: string;
  body: string;
  // Notifications with the same tag replace each other instead of piling up.
  tag: string;
  onClick?: () => void;
};

export type BrowserNotificationPermission = NotificationPermission | "unsupported";

type NotificationWindow = Window & { Notification?: typeof Notification };

function notificationAPI(target: Window): typeof Notification | null {
  const candidate = (target as NotificationWindow).Notification;
  return typeof candidate === "function" && typeof candidate.requestPermission === "function"
    ? candidate
    : null;
}

export function browserNotificationPermission(target: Window = window): BrowserNotificationPermission {
  return notificationAPI(target)?.permission ?? "unsupported";
}

export async function requestBrowserNotificationPermission(
  target: Window = window,
): Promise<BrowserNotificationPermission> {
  const api = notificationAPI(target);
  if (api === null) return "unsupported";
  if (api.permission !== "default") return api.permission;
  return api.requestPermission();
}

export function showBrowserNotification(notification: BrowserNotification, target: Window = window): boolean {
  const api = notificationAPI(target);
  if (api === null || api.permission !== "granted") return false;
  try {
    const shown = new api(notification.title, { body: notification.body, tag: notification.tag });
    if (notification.onClick !== undefined) {
      shown.onclick = () => {
        try {
          target.focus();
        } catch {
          // Some wrappers do not expose a focusable browser window.
        }
        notification.onClick?.();
        shown.close();
      };
    }
    return true;
  } catch {
    // Native wrappers and browsers can revoke delivery between checks.
    return false;
  }
}

// The browser's Notification API, for events that happen while sshc is out
// of sight: a terminal program asking for attention, a long command
// finishing, a background transfer ending. Browsers and native wrappers may
// lack the API or revoke delivery at any moment, so every call degrades to
// "not shown" instead of throwing. Android's WebView has no Notification API,
// so the Android app raises its own notifications through the bridge instead.

import {
  androidNotificationPermission,
  requestAndroidNotificationPermission,
  showAndroidNotification,
} from "../android/native";

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

// Whether notifications are Android's own, which the user allows in Android's
// settings rather than in the browser's site settings.
export function notificationsComeFromAndroid(target: Window = window): boolean {
  return androidNotificationPermission(target) !== null;
}

export function browserNotificationPermission(target: Window = window): BrowserNotificationPermission {
  return androidNotificationPermission(target) ?? notificationAPI(target)?.permission ?? "unsupported";
}

export async function requestBrowserNotificationPermission(
  target: Window = window,
): Promise<BrowserNotificationPermission> {
  const android = androidNotificationPermission(target);
  if (android !== null) return android === "default" ? requestAndroidNotificationPermission(target) : android;
  const api = notificationAPI(target);
  if (api === null) return "unsupported";
  if (api.permission !== "default") return api.permission;
  return api.requestPermission();
}

export function showBrowserNotification(notification: BrowserNotification, target: Window = window): boolean {
  if (androidNotificationPermission(target) !== null) return showAndroidNotification(notification, target);
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

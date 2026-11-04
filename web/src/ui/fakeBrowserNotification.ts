import { vi } from "vitest";

// installFakeBrowserNotification puts a browser's Notification API on window for a test, which jsdom has none of. It
// returns the notifications shown so far and takes the API away again with vi.unstubAllGlobals().
export function installFakeBrowserNotification(permission: NotificationPermission = "granted") {
  const shown: { title: string; options: NotificationOptions; onclick: (() => void) | null; close: () => void }[] = [];
  class FakeNotification {
    static permission = permission;
    static requestPermission = vi.fn(async () => permission);
    onclick: (() => void) | null = null;
    close = vi.fn();
    constructor(public title: string, public options: NotificationOptions) {
      shown.push(this);
    }
  }
  vi.stubGlobal("Notification", FakeNotification);
  return { shown };
}

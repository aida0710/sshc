import { describe, expect, it, vi } from "vitest";
import {
  browserNotificationPermission,
  requestBrowserNotificationPermission,
  showBrowserNotification,
} from "./browserNotifications";

describe("browser notification permission", () => {
  it("reports browsers without a usable Notification API as unsupported", async () => {
    const target = {} as Window;
    expect(browserNotificationPermission(target)).toBe("unsupported");
    expect(await requestBrowserNotificationPermission(target)).toBe("unsupported");
  });

  it("requests permission only while it is undecided", async () => {
    const requestPermission = vi.fn(async () => "granted");
    class FakeNotification {
      static permission: NotificationPermission = "default";
      static requestPermission = requestPermission;
    }
    const target = { Notification: FakeNotification } as unknown as Window;
    expect(await requestBrowserNotificationPermission(target)).toBe("granted");
    FakeNotification.permission = "denied";
    expect(await requestBrowserNotificationPermission(target)).toBe("denied");
    expect(requestPermission).toHaveBeenCalledOnce();
  });

  it("delivers only after permission is granted", () => {
    const created: unknown[] = [];
    class FakeNotification {
      static permission: NotificationPermission = "default";
      static requestPermission = vi.fn();
      constructor(title: string, options: NotificationOptions) { created.push({ title, options }); }
    }
    const target = { Notification: FakeNotification } as unknown as Window;
    expect(showBrowserNotification({ title: "sshc", body: "x", tag: "t" }, target)).toBe(false);
    FakeNotification.permission = "granted";
    expect(showBrowserNotification({ title: "sshc", body: "x", tag: "t" }, target)).toBe(true);
    expect(created).toEqual([{ title: "sshc", options: { body: "x", tag: "t" } }]);
  });

  it("focuses the browser and opens the exact pane when a notification is clicked", () => {
    let instance: { onclick?: () => void; close: () => void } | null = null;
    class FakeNotification {
      static permission: NotificationPermission = "granted";
      static requestPermission = vi.fn();
      onclick?: () => void;
      close = vi.fn();
      constructor() { instance = this; }
    }
    const focus = vi.fn();
    const onClick = vi.fn();
    const target = { Notification: FakeNotification, focus } as unknown as Window;
    expect(showBrowserNotification({ title: "sshc", body: "x", tag: "t", onClick }, target)).toBe(true);
    instance!.onclick?.();
    expect(focus).toHaveBeenCalledOnce();
    expect(onClick).toHaveBeenCalledOnce();
    expect(instance!.close).toHaveBeenCalledOnce();
  });
});

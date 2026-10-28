import { describe, expect, it, vi } from "vitest";
import {
  browserNotificationPermission,
  closeBrowserNotification,
  notificationsComeFromAndroid,
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

describe("taking a notification back", () => {
  it("closes the notification shown under that tag and leaves the others", () => {
    const shown: { tag: string; close: () => void }[] = [];
    class FakeNotification {
      static permission: NotificationPermission = "granted";
      static requestPermission = vi.fn();
      close = vi.fn();
      tag: string;
      constructor(_title: string, options: { tag: string }) { this.tag = options.tag; shown.push(this); }
    }
    const target = { Notification: FakeNotification } as unknown as Window;
    showBrowserNotification({ title: "sshc", body: "x", tag: "sshc-terminal-one" }, target);
    showBrowserNotification({ title: "sshc", body: "y", tag: "sshc-terminal-two" }, target);

    closeBrowserNotification("sshc-terminal-one", target);
    closeBrowserNotification("sshc-terminal-one", target);

    expect(shown[0]!.close).toHaveBeenCalledOnce();
    expect(shown[1]!.close).not.toHaveBeenCalled();
  });

  it("asks Android to cancel its notification", () => {
    const cancelNotification = vi.fn();
    const target = { sshcAndroid: { notificationPermission: () => "granted", cancelNotification } } as unknown as Window;

    closeBrowserNotification("sshc-terminal-one", target);

    expect(cancelNotification).toHaveBeenCalledWith("sshc-terminal-one");
  });
});

describe("Android app notifications", () => {
  function androidWindow(permission: string) {
    const showNotification = vi.fn(() => "");
    const target = Object.assign(new EventTarget(), {
      sshcAndroid: { notificationPermission: () => permission, requestNotificationPermission: vi.fn(), showNotification },
    }) as unknown as Window;
    return { target, showNotification };
  }

  it("uses Android's notifications instead of the browser's", () => {
    const { target, showNotification } = androidWindow("granted");
    expect(notificationsComeFromAndroid(target)).toBe(true);
    expect(browserNotificationPermission(target)).toBe("granted");
    expect(showBrowserNotification({ title: "sshc", body: "x", tag: "t" }, target)).toBe(true);
    expect(showNotification).toHaveBeenCalledWith("t", "sshc", "x");
  });

  it("returns Android's decided permission without asking again", async () => {
    const { target } = androidWindow("denied");
    expect(await requestBrowserNotificationPermission(target)).toBe("denied");
  });
});

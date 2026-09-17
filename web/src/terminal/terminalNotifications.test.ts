import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { en, ja, type MessageKey } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import {
  browserNotificationPermission,
  defaultNotificationSoundPreferences,
  loadNotificationSoundPreferences,
  nextTerminalNotification,
  playNotificationSound,
  primeNotificationSound,
  requestBrowserNotificationPermission,
  saveNotificationSoundPreferences,
  showBrowserNotification,
  useTerminalNotifications,
} from "./terminalNotifications";

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const session: TerminalSession = {
  id: "one", kind: "ssh", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "terminal", titlePinned: false },
  notificationVersion: 2,
  lastNotification: { title: "Claude Code", body: "入力を待っています", occurredAt: "2026-08-29T01:01:00Z" },
};

function translator(language: "en" | "ja"): Translate {
  const catalogue = language === "ja" ? ja : en;
  return ((key: MessageKey, values: Record<string, string> = {}) => {
    let message: string = catalogue[key];
    for (const [name, value] of Object.entries(values)) message = message.replaceAll(`{${name}}`, value);
    return message;
  }) as Translate;
}

describe("terminal notification policy", () => {
  it("does not replay the latest notification on initial load", () => {
    expect(nextTerminalNotification(translator("en"), session, undefined)).toBeNull();
  });

  it("emits only when notificationVersion advances and names the pane", () => {
    const notification = nextTerminalNotification(translator("ja"), session, 1);
    expect(notification).toEqual({ title: "API認証の修正（osaka）", body: "Claude Code\n入力を待っています" });
    expect(nextTerminalNotification(translator("ja"), session, 2)).toBeNull();
  });

  it("falls back to a generic body when the program sent no text", () => {
    const bare: TerminalSession = { ...session, lastNotification: { title: "", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translator("en"), bare, 1)?.body).toBe(en["terminal.notificationFallback"]);
    const titleOnly: TerminalSession = { ...session, lastNotification: { title: "Build finished", body: "", occurredAt: "" } };
    expect(nextTerminalNotification(translator("en"), titleOnly, 1)?.body).toBe("Build finished");
  });

  it("marks only a new notification unread and clears it when that pane is focused", async () => {
    const { lastNotification: _ignored, ...withoutNotification } = session;
    const initial: TerminalSession = { ...withoutNotification, notificationVersion: 0 };
    const open = vi.fn();
    const { result, rerender } = renderHook(
      ({ current, active }: { current: TerminalSession[]; active: string | null }) =>
        useTerminalNotifications(current, active, translator("en"), open),
      { initialProps: { current: [initial], active: null as string | null } },
    );

    rerender({ current: [{ ...initial, presentation: { ...initial.presentation!, displayTitle: "renamed" } }], active: null });
    expect(result.current.size).toBe(0);

    rerender({
      current: [{ ...initial, notificationVersion: 1, lastNotification: { title: "", body: "done", occurredAt: "" } }],
      active: null,
    });
    await waitFor(() => expect(result.current.has("one")).toBe(true));

    rerender({ current: [initial], active: "one" });
    await waitFor(() => expect(result.current.has("one")).toBe(false));
  });
});

describe("browser notification permission", () => {
  it("reports browsers without a usable Notification API as unsupported", async () => {
    const target = {} as Window;
    expect(browserNotificationPermission(target)).toBe("unsupported");
    expect(await requestBrowserNotificationPermission(target)).toBe("unsupported");
  });

  it("requests permission only while it is undecided", async () => {
    const requestPermission = vi.fn(async () => "granted" as NotificationPermission);
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

describe("notification sound preferences", () => {
  it("uses safe defaults, stores browser-local choices, and rejects unknown presets", () => {
    expect(loadNotificationSoundPreferences()).toEqual(defaultNotificationSoundPreferences);
    saveNotificationSoundPreferences({ sound: "pulse", volume: 35 });
    expect(loadNotificationSoundPreferences()).toEqual({ sound: "pulse", volume: 35 });

    window.localStorage.setItem("sshc.terminal-notification-sound.v1", JSON.stringify({ sound: "remote-url", volume: 999 }));
    expect(loadNotificationSoundPreferences()).toEqual({ sound: "bell", volume: 100 });
  });

  it("primes one reusable audio context from a user gesture", async () => {
    const start = vi.fn();
    const resume = vi.fn(async function (this: { state: AudioContextState }) { this.state = "running"; });
    class FakeAudioContext {
      state: AudioContextState = "suspended";
      currentTime = 0;
      destination = {};
      resume = resume;
      createGain = () => ({
        gain: { setValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() },
        connect: vi.fn(),
      });
      createOscillator = () => ({
        type: "sine" as OscillatorType,
        frequency: { value: 0 },
        connect: vi.fn(),
        start,
        stop: vi.fn(),
      });
    }
    const target = { AudioContext: FakeAudioContext } as unknown as Window;

    expect(primeNotificationSound(target)).toBe(true);
    await waitFor(() => expect(resume).toHaveBeenCalledOnce());
    expect(playNotificationSound(defaultNotificationSoundPreferences, target)).toBe(true);
    expect(start).toHaveBeenCalledTimes(2);
    expect(playNotificationSound({ sound: "none", volume: 60 }, target)).toBe(false);
  });
});

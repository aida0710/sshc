import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/integrations";
import { en, ja, type MessageKey } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import {
  browserNotificationPermission,
  defaultAgentSoundPreferences,
  loadAgentSoundPreferences,
  nextAgentNotification,
  playAgentSound,
  primeAgentSound,
  requestBrowserNotificationPermission,
  saveAgentSoundPreferences,
  showBrowserNotification,
  useAgentNotifications,
} from "./agentNotifications";

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const session: TerminalSession = {
  id: "one", kind: "ssh", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "agent", titlePinned: false },
  agent: {
    kind: "codex", state: "attention", resumable: true,
    observationVersion: 3, signalVersion: 2,
    lastSignal: { kind: "attention", occurredAt: "2026-08-29T01:01:00Z" },
  },
};

function translator(language: "en" | "ja"): Translate {
  const catalogue = language === "ja" ? ja : en;
  return ((key: MessageKey, values: Record<string, string> = {}) => {
    let message: string = catalogue[key];
    for (const [name, value] of Object.entries(values)) message = message.replaceAll(`{${name}}`, value);
    return message;
  }) as Translate;
}

describe("agent notification policy", () => {
  it("does not replay the latest signal on initial load", () => {
    expect(nextAgentNotification(translator("en"), session, undefined)).toBeNull();
  });

  it("emits only when signalVersion advances and keeps private fields out", () => {
    const notification = nextAgentNotification(translator("ja"), session, 1);
    expect(notification?.body).toBe("API認証の修正（osaka）が入力を待っています");
    expect(notification?.body).not.toContain("thread");
    expect(nextAgentNotification(translator("ja"), session, 2)).toBeNull();
  });

  it("treats a hook-provided ready completion as a completed notification", () => {
    const completed: TerminalSession = {
      ...session,
      agent: {
        ...session.agent!, state: "ready", signalVersion: 3,
        lastSignal: { kind: "completed", occurredAt: "2026-08-29T01:02:00Z" },
      },
    };
    expect(nextAgentNotification(translator("en"), completed, 2)?.kind).toBe("completed");
    expect(nextAgentNotification(translator("en"), completed, 2)?.body).toBe("API認証の修正（osaka） has finished");
  });

  it("marks only a new hook signal unread and clears it when that pane is focused", async () => {
    const initial: TerminalSession = {
      ...session,
      agent: {
        kind: "codex", state: "working", resumable: true,
        observationVersion: 3, signalVersion: 0,
      },
    };
    const open = vi.fn();
    const { result, rerender } = renderHook(
      ({ current, active }: { current: TerminalSession[]; active: string | null }) =>
        useAgentNotifications(current, active, translator("en"), open),
      { initialProps: { current: [initial], active: null as string | null } },
    );

    rerender({
      current: [{ ...initial, agent: { ...initial.agent!, state: "attention" } }],
      active: null,
    });
    expect(result.current.size).toBe(0);

    rerender({
      current: [{
        ...initial,
        agent: {
          ...initial.agent!, state: "ready", signalVersion: 1,
          lastSignal: { kind: "completed", occurredAt: "2026-08-29T01:02:00Z" },
        },
      }],
      active: null,
    });
    await waitFor(() => expect(result.current.get("one")).toBe("completed"));

    rerender({ current: [initial], active: "one" });
    await waitFor(() => expect(result.current.has("one")).toBe(false));
  });
});

describe("browser notification permission", () => {
  it("reports browsers without a usable Notification API as unsupported", async () => {
    const target = {} as Window;
    expect(browserNotificationPermission(target)).toBe("unsupported");
    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("unsupported");
  });

  it("requests permission only while it is undecided", async () => {
    class FakeNotification {
      static permission: NotificationPermission = "default";
      static requestPermission = vi.fn(async () => {
        FakeNotification.permission = "granted";
        return "granted" as const;
      });
    }
    const target = { Notification: FakeNotification } as unknown as Window;

    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("granted");
    await expect(requestBrowserNotificationPermission(target)).resolves.toBe("granted");
    expect(FakeNotification.requestPermission).toHaveBeenCalledTimes(1);
  });

  it("delivers only after permission is granted", () => {
    const delivered: Array<{ title: string; options: NotificationOptions | undefined }> = [];
    class FakeNotification {
      static permission: NotificationPermission = "denied";
      static requestPermission = vi.fn();

      constructor(title: string, options?: NotificationOptions) {
        delivered.push({ title, options });
      }
    }
    const target = { Notification: FakeNotification } as unknown as Window;
    const message = { title: "sshc", body: "ready", tag: "test" };

    expect(showBrowserNotification(message, target)).toBe(false);
    FakeNotification.permission = "granted";
    expect(showBrowserNotification(message, target)).toBe(true);
    expect(delivered).toEqual([{ title: "sshc", options: { body: "ready", tag: "test" } }]);
  });

  it("focuses the browser and opens the exact pane when a notification is clicked", () => {
    const delivered: FakeNotification[] = [];
    class FakeNotification {
      static permission: NotificationPermission = "granted";
      static requestPermission = vi.fn();
      onclick: (() => void) | null = null;
      close = vi.fn();

      constructor() {
        delivered.push(this);
      }
    }
    const focus = vi.fn();
    const open = vi.fn();
    const target = { Notification: FakeNotification, focus } as unknown as Window;

    expect(showBrowserNotification({ title: "sshc", body: "done", tag: "agent", onClick: open }, target)).toBe(true);
    delivered[0]?.onclick?.();

    expect(focus).toHaveBeenCalledOnce();
    expect(open).toHaveBeenCalledOnce();
    expect(delivered[0]?.close).toHaveBeenCalledOnce();
  });
});

describe("Agent notification sound preferences", () => {
  it("uses safe defaults, stores browser-local choices, and rejects unknown presets", () => {
    expect(loadAgentSoundPreferences()).toEqual(defaultAgentSoundPreferences);
    saveAgentSoundPreferences({ attention: "pulse", completed: "none", volume: 35 });
    expect(loadAgentSoundPreferences()).toEqual({ attention: "pulse", completed: "none", volume: 35 });

    window.localStorage.setItem("sshc.agent-notification-sounds.v1", JSON.stringify({
      attention: "remote-url", completed: "bell", volume: 999,
    }));
    expect(loadAgentSoundPreferences()).toEqual({ attention: "bell", completed: "bell", volume: 100 });
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

    expect(primeAgentSound(target)).toBe(true);
    await waitFor(() => expect(resume).toHaveBeenCalledOnce());
    expect(playAgentSound("completed", defaultAgentSoundPreferences, target)).toBe(true);
    expect(start).toHaveBeenCalledTimes(2);
  });
});

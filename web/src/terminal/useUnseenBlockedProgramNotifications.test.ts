import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "../android/fakeAndroidBridge";
import type { AndroidBridge } from "../android/native";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { en } from "../i18n/messages";
import { installFakeBrowserNotification } from "../ui/fakeBrowserNotification";
import { terminalNotificationTag } from "./notificationTags";
import type { SeenProgramStatus } from "./programStatus";
import { useUnseenBlockedProgramNotifications } from "./useUnseenBlockedProgramNotifications";

vi.mock("./notificationSound", () => ({ playNotificationSound: vi.fn() }));

const translate = ((key) => en[key]) as Translate;
const waiting: TerminalSession = {
  id: "one", kind: "ssh", workspaceId: "default", alias: "orchid", title: "Claude Code",
  startedAt: "2026-11-04T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "Claude Code", titleSource: "terminal", titlePinned: false },
  programStatus: { state: "blocked", kind: "permission", message: "approve Bash", version: 2 },
};
const working: TerminalSession = { ...waiting, programStatus: { state: "working", version: 3 } };
const nothingSeen: SeenProgramStatus = new Map();

function setPageHidden(hidden: boolean) {
  vi.spyOn(document, "hidden", "get").mockReturnValue(hidden);
  document.dispatchEvent(new Event("visibilitychange"));
}

function renderWith(session: TerminalSession, seen: SeenProgramStatus, onOpenSession: (id: string) => void = vi.fn()) {
  return renderHook(() => useUnseenBlockedProgramNotifications([session], seen, { t: translate, onOpenSession }));
}

describe("useUnseenBlockedProgramNotifications", () => {
  let bridge: AndroidBridge;

  beforeEach(() => {
    bridge = fakeAndroidBridge();
    window.sshcAndroid = bridge;
  });

  afterEach(() => {
    delete window.sshcAndroid;
    vi.restoreAllMocks();
  });

  it("notifies on leaving about a program that started waiting out of the person's sight", () => {
    renderWith(waiting, nothingSeen);
    expect(bridge.showBlockedProgramNotification).not.toHaveBeenCalled();
    setPageHidden(true);
    expect(bridge.showBlockedProgramNotification).toHaveBeenCalledTimes(1);
    const [tag, title, body] = vi.mocked(bridge.showBlockedProgramNotification).mock.calls[0] as unknown as [string, string, string];
    expect(title).toBe("orchid（Claude Code）");
    expect(body).toContain("approve Bash");
    expect(tag).toBe(terminalNotificationTag("one"));
  });

  it("stays quiet about a wait the person saw", () => {
    renderWith(waiting, new Map([["one", 2]]));
    setPageHidden(true);
    expect(bridge.showBlockedProgramNotification).not.toHaveBeenCalled();
  });

  it("stays quiet about a program that is not waiting", () => {
    renderWith(working, nothingSeen);
    setPageHidden(true);
    expect(bridge.showBlockedProgramNotification).not.toHaveBeenCalled();
  });

  it("announces one wait once, however many times the person comes and goes", () => {
    renderWith(waiting, nothingSeen);
    setPageHidden(true);
    setPageHidden(false);
    setPageHidden(true);
    expect(bridge.showBlockedProgramNotification).toHaveBeenCalledTimes(1);
  });

  it("does not take Android's notification shade for leaving the app", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
    renderWith(waiting, nothingSeen);
    window.dispatchEvent(new Event("blur"));
    expect(bridge.showBlockedProgramNotification).not.toHaveBeenCalled();
  });

  describe("in a browser", () => {
    beforeEach(() => {
      delete window.sshcAndroid;
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it("notifies when the tab goes to the background, and the notification opens the session", () => {
      const { shown } = installFakeBrowserNotification();
      const open = vi.fn();
      renderWith(waiting, nothingSeen, open);

      setPageHidden(true);

      expect(shown).toHaveLength(1);
      shown[0]?.onclick?.();
      expect(open).toHaveBeenCalledExactlyOnceWith("one");
    });

    it("notifies when another window takes the focus", () => {
      const { shown } = installFakeBrowserNotification();
      vi.spyOn(document, "hasFocus").mockReturnValue(false);
      renderWith(waiting, nothingSeen);

      window.dispatchEvent(new Event("blur"));

      expect(shown).toHaveLength(1);
    });

    it("does not count coming back to the tab as leaving", () => {
      const { shown } = installFakeBrowserNotification();
      vi.spyOn(document, "hasFocus").mockReturnValue(false);
      renderWith(waiting, nothingSeen);

      setPageHidden(false);

      expect(shown).toEqual([]);
    });
  });
});

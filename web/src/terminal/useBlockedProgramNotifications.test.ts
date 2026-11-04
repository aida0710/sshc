import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "../android/fakeAndroidBridge";
import type { AndroidBridge } from "../android/native";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { en } from "../i18n/messages";
import { installFakeBrowserNotification } from "../ui/fakeBrowserNotification";
import { useBlockedProgramNotifications } from "./useBlockedProgramNotifications";

vi.mock("./notificationSound", () => ({ playNotificationSound: vi.fn() }));

const translate = ((key) => en[key]) as Translate;
const working: TerminalSession = {
  id: "one", kind: "ssh", workspaceId: "default", alias: "orchid", title: "Claude Code",
  startedAt: "2026-11-04T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "Claude Code", titleSource: "terminal", titlePinned: false },
  programStatus: { state: "working", version: 1 },
};
const waiting: TerminalSession = {
  ...working, programStatus: { state: "blocked", kind: "permission", message: "approve Bash", version: 2 },
};

function renderWith(initial: TerminalSession, enabled = true, onOpenSession: (id: string) => void = vi.fn()) {
  return renderHook(
    ({ session, on }: { session: TerminalSession; on: boolean }) => useBlockedProgramNotifications([session], { enabled: on, t: translate, onOpenSession }),
    { initialProps: { session: initial, on: enabled } },
  );
}

describe("useBlockedProgramNotifications", () => {
  let bridge: AndroidBridge;

  beforeEach(() => {
    bridge = fakeAndroidBridge();
    window.sshcAndroid = bridge;
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
  });

  afterEach(() => {
    delete window.sshcAndroid;
    vi.restoreAllMocks();
  });

  it("notifies a person who is away when a program starts waiting, as the session's row says it", () => {
    const { rerender } = renderWith(working);

    rerender({ session: waiting, on: true });

    expect(bridge.showNotification).toHaveBeenCalledExactlyOnceWith("sshc-terminal-one", "orchid（Claude Code）", "Needs permission · approve Bash");
  });

  it("says nothing new while the program goes on waiting", () => {
    const { rerender } = renderWith(working);
    rerender({ session: waiting, on: true });
    rerender({ session: { ...waiting, programStatus: { state: "blocked", message: "other words", version: 3 } }, on: true });

    expect(bridge.showNotification).toHaveBeenCalledOnce();
  });

  it("takes the notification back once the program stops waiting", () => {
    const { rerender } = renderWith(working);
    rerender({ session: waiting, on: true });
    expect(bridge.cancelNotification).not.toHaveBeenCalled();

    rerender({ session: working, on: true });

    expect(bridge.cancelNotification).toHaveBeenCalledExactlyOnceWith("sshc-terminal-one");
  });

  it("takes a program already waiting at first sight as history", () => {
    renderWith(waiting);
    expect(bridge.showNotification).not.toHaveBeenCalled();
  });

  it("leaves a person looking at sshc to the session's row", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const { rerender } = renderWith(working);

    rerender({ session: waiting, on: true });

    expect(bridge.showNotification).not.toHaveBeenCalled();
  });

  it("does nothing when another part of the app notifies", () => {
    const { rerender } = renderWith(working, false);

    rerender({ session: waiting, on: false });

    expect(bridge.showNotification).not.toHaveBeenCalled();
  });

  describe("in a browser", () => {
    beforeEach(() => {
      delete window.sshcAndroid;
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it("shows a browser notification that opens the session", () => {
      const { shown } = installFakeBrowserNotification();
      const open = vi.fn();
      const { rerender } = renderWith(working, true, open);

      rerender({ session: waiting, on: true });

      expect(shown).toHaveLength(1);
      expect(shown[0]?.title).toBe("orchid（Claude Code）");
      expect(shown[0]?.options.body).toContain("approve Bash");
      shown[0]?.onclick?.();
      expect(open).toHaveBeenCalledExactlyOnceWith("one");
    });

    it("takes the browser notification back once the program stops waiting", () => {
      const { shown } = installFakeBrowserNotification();
      const { rerender } = renderWith(working);
      rerender({ session: waiting, on: true });

      rerender({ session: working, on: true });

      expect(shown[0]?.close).toHaveBeenCalledOnce();
    });
  });
});

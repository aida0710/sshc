import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { en } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import { useTerminalNotifications } from "./useTerminalNotifications";
import { fakeAndroidBridge } from "../android/fakeAndroidBridge";

const initial: TerminalSession = {
  id: "one", kind: "ssh", workspaceId: "default", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "terminal", titlePinned: false },
  notificationVersion: 0,
};
const translate = ((key) => en[key]) as Translate;

// jsdom never gives the document focus; a person looking at sshc does.
beforeEach(() => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useTerminalNotifications", () => {
  it("marks only a new notification unread and clears it when that pane is focused", async () => {
    const open = vi.fn();
    const { result, rerender } = renderHook(
      ({ current, active }: { current: TerminalSession[]; active: string | null }) =>
        useTerminalNotifications(current, active, translate, open),
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

  describe("in the Android app", () => {
    afterEach(() => {
      delete window.sshcAndroid;
    });

    it("opens the session of a notification tapped before the session list loaded", async () => {
      let pending = "sshc-terminal-one";
      window.sshcAndroid = fakeAndroidBridge({
        takeOpenedNotification: () => { const tag = pending; pending = ""; return tag; },
      });
      const open = vi.fn();
      const { rerender } = renderHook(
        ({ current }: { current: TerminalSession[] }) => useTerminalNotifications(current, null, translate, open),
        { initialProps: { current: [] as TerminalSession[] } },
      );
      expect(open).not.toHaveBeenCalled();

      rerender({ current: [initial] });
      await waitFor(() => expect(open).toHaveBeenCalledExactlyOnceWith("one"));
      rerender({ current: [{ ...initial }] });
      expect(open).toHaveBeenCalledOnce();
    });
  });

  describe("while the person is away", () => {
    afterEach(() => {
      delete window.sshcAndroid;
    });

    const notified = (version: number): TerminalSession => ({
      ...initial, notificationVersion: version, lastNotification: { title: "Claude Code", body: "waiting", occurredAt: "" },
    });

    it("notifies about the session in view when another window has the focus", async () => {
      const bridge = fakeAndroidBridge();
      window.sshcAndroid = bridge;
      vi.spyOn(document, "hasFocus").mockReturnValue(false);
      const { result, rerender } = renderHook(
        ({ current }: { current: TerminalSession[] }) => useTerminalNotifications(current, "one", translate, vi.fn()),
        { initialProps: { current: [initial] } },
      );

      rerender({ current: [notified(1)] });

      await waitFor(() => expect(result.current.has("one")).toBe(true));
      expect(bridge.showNotification).toHaveBeenCalledWith("sshc-terminal-one", expect.any(String), expect.stringContaining("waiting"));
    });

    it("takes the notification back once the session is seen", async () => {
      const bridge = fakeAndroidBridge();
      window.sshcAndroid = bridge;
      const focused = vi.spyOn(document, "hasFocus").mockReturnValue(false);
      const { rerender } = renderHook(
        ({ current, active }: { current: TerminalSession[]; active: string | null }) =>
          useTerminalNotifications(current, active, translate, vi.fn()),
        { initialProps: { current: [initial], active: null as string | null } },
      );
      rerender({ current: [notified(1)], active: null });
      await waitFor(() => expect(bridge.showNotification).toHaveBeenCalled());
      expect(bridge.cancelNotification).not.toHaveBeenCalled();

      focused.mockReturnValue(true);
      rerender({ current: [notified(1)], active: "one" });

      await waitFor(() => expect(bridge.cancelNotification).toHaveBeenCalledWith("sshc-terminal-one"));
      expect(bridge.cancelNotification).toHaveBeenCalledWith("sshc-command-one");
    });

    it("takes back the notifications of a session that was closed", async () => {
      const bridge = fakeAndroidBridge();
      window.sshcAndroid = bridge;
      vi.spyOn(document, "hasFocus").mockReturnValue(false);
      const { rerender } = renderHook(
        ({ current }: { current: TerminalSession[] }) => useTerminalNotifications(current, null, translate, vi.fn()),
        { initialProps: { current: [initial] } },
      );
      rerender({ current: [notified(1)] });
      await waitFor(() => expect(bridge.showNotification).toHaveBeenCalled());

      rerender({ current: [] });

      await waitFor(() => expect(bridge.cancelNotification).toHaveBeenCalledWith("sshc-terminal-one"));
    });
  });
});

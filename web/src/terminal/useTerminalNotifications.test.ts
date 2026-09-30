import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { en } from "../i18n/messages";
import type { Translate } from "../i18n/context";
import { useTerminalNotifications } from "./useTerminalNotifications";

const initial: TerminalSession = {
  id: "one", kind: "ssh", alias: "osaka", title: "API認証の修正",
  startedAt: "2026-08-29T01:00:00Z", state: "connected", problem: "",
  presentation: { displayTitle: "API認証の修正", titleSource: "terminal", titlePinned: false },
  notificationVersion: 0,
};
const translate = ((key) => en[key]) as Translate;

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
});

import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { forgetAllDismissedTerminalNotices, forgetDismissedTerminalNotices, useTerminalNoticeDismissal } from "./dismissedTerminalNotices";
import { TerminalNoticeCard, type NoticeFocus } from "./TerminalNoticeCard";
import { terminalNoticeFor } from "./terminalNotice";
import type { HostKeyAnswer } from "./useHostKeyAnswer";
import type { SessionRecovery } from "./useSessionRecovery";

afterEach(() => forgetAllDismissedTerminalNotices());

const session: TerminalSession = {
  id: "s1", kind: "ssh", alias: "web-01", workspaceId: "default", title: "web-01", startedAt: "2026-11-02T09:00:00Z", state: "connected", problem: "",
};
const lost: TerminalSession = { ...session, state: "exited", exited: { code: -1, signal: "", at: "2026-11-02T09:30:00Z", connectionLost: true } };

function recovery(overrides: Partial<SessionRecovery> = {}): SessionRecovery {
  return { reconnect: vi.fn(), reconnectBusy: false, reconnectFailed: false, stopReconnecting: vi.fn(), stopBusy: false, ...overrides };
}

type CardOptions = { onCloseSession?: () => void; onDismiss?: () => void; answers?: HostKeyAnswer; focus?: NoticeFocus };

function renderCard(
  shown: TerminalSession,
  actions: SessionRecovery,
  { onCloseSession = vi.fn(), onDismiss = vi.fn(), answers = { answer: vi.fn(), busy: false }, focus = { take: false, giveBack: vi.fn() } }: CardOptions = {},
) {
  const notice = terminalNoticeFor(shown, { phase: "live" });
  if (notice === null) throw new Error("no card for this session");
  const view = render(<TerminalNoticeCard notice={notice} session={shown} title="web-01" recovery={actions} hostKeyAnswer={answers} focus={focus} onLinkNow={vi.fn()} onCloseSession={onCloseSession} onDismiss={onDismiss} />);
  return { onCloseSession, onDismiss, answers, focus, view };
}

describe("TerminalNoticeCard", () => {
  it("offers Reconnect and Close for a lost SSH connection, under the reason", async () => {
    const actions = recovery();
    const { onCloseSession } = renderCard(lost, actions);
    const card = screen.getByRole("region", { name: "The SSH connection was lost." });
    expect(card).toHaveTextContent("Stopped");
    await userEvent.click(screen.getByRole("button", { name: "Reconnect" }));
    expect(actions.reconnect).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onCloseSession).toHaveBeenCalledOnce();
  });

  it("is not a live region, so the strip above is what a screen reader reads", () => {
    renderCard(lost, recovery());
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers only Close for a local shell that failed", () => {
    const { alias: _alias, ...shell } = session;
    renderCard({ ...shell, kind: "shell", state: "exited", exited: { code: 3, signal: "", at: "2026-11-02T09:30:00Z" } }, recovery());
    expect(screen.getByRole("button", { name: "Close" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Reconnect" })).toBeNull();
  });

  it("counts down to the next automatic attempt and stops it", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    try {
      vi.setSystemTime(new Date("2026-11-02T09:31:00Z"));
      const actions = recovery();
      renderCard({ ...session, state: "reconnecting", reconnect: { attempt: 2, limit: 5, retryAt: "2026-11-02T09:31:08Z", problem: "reconnect_failed" } }, actions);
      expect(screen.getByText("Reconnecting (2/5)")).toBeVisible();
      expect(screen.getByRole("region", { name: "The connection to web-01 was lost" })).toHaveTextContent("The next try starts in 8 s.");
      await act(async () => { screen.getByRole("button", { name: "Stop reconnecting" }).click(); });
      expect(actions.stopReconnecting).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  it("asks about an unknown host key with its fingerprint, and answers with the fingerprint asked about", async () => {
    const asking: TerminalSession = {
      ...session, state: "connecting",
      hostKeyQuestion: { alias: "web-01", hostName: "10.0.2.15", keyType: "ssh-ed25519", fingerprint: "SHA256:ZncdGvxXvUJOYNkbZuT4Ew" },
    };
    const { answers } = renderCard(asking, recovery());
    const card = screen.getByRole("region", { name: "Check the host key of web-01" });
    expect(card).toHaveTextContent("First connection to this host");
    expect(card).toHaveTextContent("ED25519 SHA256:ZncdGvxXvUJOYNkbZuT4Ew");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(answers.answer).toHaveBeenLastCalledWith("SHA256:ZncdGvxXvUJOYNkbZuT4Ew", false);
    await userEvent.click(screen.getByRole("button", { name: "Connect" }));
    expect(answers.answer).toHaveBeenLastCalledWith("SHA256:ZncdGvxXvUJOYNkbZuT4Ew", true);
  });

  it("hides with its own button", async () => {
    const { onDismiss } = renderCard(lost, recovery());
    await userEvent.click(screen.getByRole("button", { name: "Hide this notice" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it("takes the focus itself when it appears, not a button an Enter would press", () => {
    renderCard(lost, recovery(), { focus: { take: true, giveBack: vi.fn() } });
    expect(screen.getByRole("region", { name: "The SSH connection was lost." })).toHaveFocus();
  });

  it("leaves the focus where it is when told not to take it", () => {
    renderCard(lost, recovery());
    expect(screen.getByRole("region", { name: "The SSH connection was lost." })).not.toHaveFocus();
  });

  it("hides with Escape and gives the focus back when it goes away with the focus in it", async () => {
    const giveBack = vi.fn();
    const { onDismiss, view } = renderCard(lost, recovery(), { focus: { take: true, giveBack } });
    await userEvent.keyboard("{Escape}");
    expect(onDismiss).toHaveBeenCalledOnce();
    view.unmount();
    expect(giveBack).toHaveBeenCalledOnce();
  });

  it("does not take the focus back to give away when the card goes without it", () => {
    const giveBack = vi.fn();
    const { view } = renderCard(lost, recovery(), { focus: { take: false, giveBack } });
    view.unmount();
    expect(giveBack).not.toHaveBeenCalled();
  });
});

describe("useTerminalNoticeDismissal", () => {
  it("keeps a card hidden when the view is shown again, until the session works again", () => {
    const key = terminalNoticeFor(lost, { phase: "live" })!.key;
    const first = renderHook(() => useTerminalNoticeDismissal(key));
    act(() => first.result.current.dismiss());
    expect(first.result.current.dismissed).toBe(true);
    first.unmount();

    expect(renderHook(() => useTerminalNoticeDismissal(key)).result.current.dismissed).toBe(true);
    forgetDismissedTerminalNotices(session.id);
    expect(renderHook(() => useTerminalNoticeDismissal(key)).result.current.dismissed).toBe(false);
  });
});

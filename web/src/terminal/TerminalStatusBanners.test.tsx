import { act, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { TerminalStatusBanners } from "./TerminalStatusBanners";
import type { StreamLink } from "./streamLink";
import type { SessionRecovery } from "./useSessionRecovery";

const idleRecovery: SessionRecovery = {
  reconnect: () => undefined, reconnectBusy: false, reconnectFailed: false, stopReconnecting: () => undefined, stopBusy: false,
};

const session: TerminalSession = {
  id: "a", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-08-14T09:00:00Z", state: "connected", problem: "",
};

// スクリーンリーダーが live 領域から読む文。aria-hidden の中は読まれない。
function spokenText(node: Node): string {
  if (node instanceof Element && node.getAttribute("aria-hidden") === "true") return "";
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  return [...node.childNodes].map(spokenText).join("");
}

function Banners({ link }: { link: StreamLink }) {
  return <TerminalStatusBanners session={session} problem="" link={link} onLinkNow={() => undefined} onLinkStop={() => undefined} />;
}

it("announces a dropped link once per attempt instead of every second of the countdown", () => {
  const { rerender } = render(<Banners link={{ phase: "waiting", attempt: 2, seconds: 4 }} />);
  const status = screen.getByRole("status");
  const firstSecond = spokenText(status);

  rerender(<Banners link={{ phase: "waiting", attempt: 2, seconds: 3 }} />);

  expect(screen.getByText(/Attempt 2 in 3s/)).toBeVisible();
  expect(firstSecond).toContain("Waiting to make attempt 2");
  expect(spokenText(status)).toBe(firstSecond);

  rerender(<Banners link={{ phase: "waiting", attempt: 3, seconds: 2 }} />);

  expect(spokenText(status)).toContain("Waiting to make attempt 3");
});

it("counts down to the next reconnect attempt and then says it is connecting", () => {
  vi.useFakeTimers();
  try {
    vi.setSystemTime(new Date("2026-10-27T05:00:00Z"));
    const reconnecting: TerminalSession = {
      ...session, kind: "ssh", alias: "db", state: "reconnecting",
      reconnect: { attempt: 2, limit: 5, retryAt: "2026-10-27T05:00:03Z", problem: "reconnect_failed" },
    };
    render(<TerminalStatusBanners session={reconnecting} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);
    expect(screen.getByText("reconnecting 2/5 · next try in 3 s")).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(3_000); });
    expect(screen.getByText("reconnecting 2/5 · connecting")).toBeInTheDocument();
  } finally {
    vi.useRealTimers();
  }
});

it("says a lost SSH connection ended the session, in red, rather than exit status -1", () => {
  const lost: TerminalSession = {
    ...session, kind: "ssh", alias: "bastion", state: "exited",
    exited: { code: -1, signal: "", at: "2026-10-28T09:00:00Z", connectionLost: true },
  };
  render(<TerminalStatusBanners session={lost} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);

  const message = screen.getByText("The SSH connection was lost.");
  expect(message).toHaveClass("text-danger");
  expect(screen.queryByText(/status -1/)).toBeNull();
});

// After a dropped connection the reason reconnecting stopped is the ended row itself, with Reconnect beside
// it: one row, not the reason above a row that says the connection was lost.
it("says why an ended session stopped reconnecting once, in its ended row", () => {
  for (const [problem, reason] of [
    ["reconnect_exhausted", "The SSH connection was lost and did not come back within the reconnect attempts."],
    ["reconnect_stopped", "The SSH connection was lost and automatic reconnecting was stopped."],
  ] as const) {
    const ended: TerminalSession = {
      ...session, kind: "ssh", alias: "bastion", state: "exited", problem,
      exited: { code: -1, signal: "", at: "2026-10-28T09:00:00Z", connectionLost: true },
    };
    const { unmount } = render(<TerminalStatusBanners session={ended} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} recovery={idleRecovery} />);
    expect(screen.getByRole("alert")).toHaveTextContent(reason);
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.queryByText(/^The SSH connection was lost\.$/)).toBeNull();
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeVisible();
    unmount();
  }
});

it("never shows the engine's unknown exit status as a status", () => {
  const unknown: TerminalSession = { ...session, state: "exited", exited: { code: -1, signal: "", at: "2026-10-28T09:00:00Z" } };
  render(<TerminalStatusBanners session={unknown} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);
  expect(screen.getByText("The program exited. Its exit status could not be read.")).toBeVisible();
  expect(screen.queryByText(/-1/)).toBeNull();
});

it("shows a clean exit quietly and a failed one in red", () => {
  const exitedWith = (code: number): TerminalSession => ({ ...session, state: "exited", exited: { code, signal: "", at: "2026-10-28T09:00:00Z" } });
  const { rerender } = render(<TerminalStatusBanners session={exitedWith(0)} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);
  expect(screen.getByText(/exited with status 0/)).toHaveClass("text-ink-muted");

  rerender(<TerminalStatusBanners session={exitedWith(3)} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);
  expect(screen.getByText(/exited with status 3/)).toHaveClass("text-danger");
});

it("says in red that a session ended as soon as it opened, even with status 0", () => {
  const rightAway: TerminalSession = { ...session, state: "exited", exited: { code: 0, signal: "", at: "2026-08-14T09:00:00.080Z" } };
  render(<TerminalStatusBanners session={rightAway} problem="" link={{ phase: "live" }} onLinkNow={() => undefined} onLinkStop={() => undefined} />);

  expect(screen.getByText(/exited with status 0 as soon as it started/)).toHaveClass("text-danger");
});

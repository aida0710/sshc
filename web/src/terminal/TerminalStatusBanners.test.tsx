import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { TerminalStatusBanners } from "./TerminalStatusBanners";
import type { StreamLink } from "./streamLink";

const session: TerminalSession = {
  id: "a", kind: "shell", title: "zsh", startedAt: "2026-08-14T09:00:00Z", state: "connected", problem: "",
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

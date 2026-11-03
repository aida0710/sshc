import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { LanguageProvider } from "../i18n/context";
import { SessionSettingsSection, type SessionSettingsState } from "./SessionSettingsSection";

const session = (id: string, title: string): TerminalSession => ({
  id, kind: "ssh", workspaceId: "default", alias: "web-01", title, startedAt: "2026-11-03T08:00:00Z", state: "connected", problem: "",
});

function renderWith(sessions: TerminalSession[]) {
  const state: SessionSettingsState = { sessions, busy: false, closeAll: vi.fn() };
  render(<LanguageProvider><SessionSettingsSection terminalSessions={state} showHeading /></LanguageProvider>);
}

describe("SessionSettingsSection", () => {
  it("says which sessions are open, not only how many", () => {
    renderWith([session("one", "web-01 logs"), session("two", "deploy")]);

    const open = screen.getByRole("list", { name: "Open sessions" });
    expect(within(open).getByText("web-01 logs")).toBeVisible();
    expect(within(open).getByText("deploy")).toBeVisible();
    expect(screen.getByText("2 open")).toBeVisible();
  });

  it("has no list while nothing is open", () => {
    renderWith([]);
    expect(screen.queryByRole("list", { name: "Open sessions" })).toBeNull();
  });
});

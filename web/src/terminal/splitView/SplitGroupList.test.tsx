import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../../api/terminalSessions";
import type { SplitGroup } from "./splitGroups";
import { SplitGroupList } from "./SplitGroupList";

const session = (id: string, extra: Partial<TerminalSession> = {}): TerminalSession => ({
  id, kind: "shell", workspaceId: "default", title: id, startedAt: "2026-10-26T00:00:00Z", state: "connected", problem: "", ...extra,
});

// 分割 1: 左に fix、右を上下に log と build。
const group: SplitGroup = {
  number: 1,
  view: {
    root: {
      direction: "horizontal", ratio: 50,
      first: { sessionId: "fix" },
      second: { direction: "vertical", ratio: 50, first: { sessionId: "log" }, second: { sessionId: "build" } },
    },
    activeSessionId: "fix",
    focusMode: false,
    focusRevision: 0,
  },
};

const sessions = [
  session("fix", { title: "sshc の改修" }),
  session("log", { title: "ログ確認" }),
  session("build", { kind: "ssh", alias: "build-01", title: "ビルド", state: "reconnecting", reconnect: { attempt: 2, limit: 5, retryAt: "2026-10-26T00:00:05Z", problem: "" } }),
];

function renderList({ selected = 1 }: { selected?: number | null } = {}) {
  const props = { onSelect: vi.fn(), onSelectSession: vi.fn(), onDissolve: vi.fn() };
  render(<SplitGroupList groups={[group]} sessions={sessions} selected={selected} unreadBySession={new Set(["log"])} {...props} />);
  return props;
}

describe("SplitGroupList", () => {
  it("lists the sessions a split holds under it, with where each connects", () => {
    renderList();

    const members = screen.getByRole("list", { name: "Sessions in Split 1" });
    const rows = within(members).getAllByRole("button");
    expect(rows.map((row) => row.getAttribute("aria-label"))).toEqual(["sshc の改修 in Split 1", "ログ確認 in Split 1", "ビルド in Split 1"]);
    expect(rows[0]).toHaveTextContent("localhost");
    expect(rows[2]).toHaveTextContent("reconnecting 2/5");
  });

  it("shows only the number of sessions next to the group's name", () => {
    renderList();

    const groupButton = screen.getByRole("button", { name: "Split 1" });
    expect(groupButton).toHaveTextContent(/Split 1\s*3$/);
  });

  it("draws the arrangement of the split as a small map", () => {
    renderList();

    const map = screen.getByRole("button", { name: "Split 1" }).querySelector("[data-split-layout-map]");
    expect(map?.children).toHaveLength(3);
  });

  it("marks the session typing goes to while its split is shown", () => {
    renderList();

    const members = within(screen.getByRole("list", { name: "Sessions in Split 1" })).getAllByRole("button");
    expect(members.map((row) => row.getAttribute("aria-current"))).toEqual(["true", null, null]);
  });

  it("marks no session while another split or a single session is shown", () => {
    renderList({ selected: null });

    const members = within(screen.getByRole("list", { name: "Sessions in Split 1" })).getAllByRole("button");
    expect(members.every((row) => !row.hasAttribute("aria-current"))).toBe(true);
  });

  it("opens a session of the split, and the split itself, from the rows", async () => {
    const props = renderList();

    await userEvent.click(screen.getByRole("button", { name: "ログ確認 in Split 1" }));
    await userEvent.click(screen.getByRole("button", { name: "Split 1" }));

    expect(props.onSelectSession).toHaveBeenCalledWith("log");
    expect(props.onSelect).toHaveBeenCalledWith(1);
  });

  it("dissolves the split from its menu", async () => {
    const props = renderList();

    await userEvent.click(screen.getByRole("button", { name: "Actions for Split 1" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Dissolve" }));

    expect(props.onDissolve).toHaveBeenCalledWith(1);
  });
});

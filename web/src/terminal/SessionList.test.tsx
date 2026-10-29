import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SessionList } from "./SessionList";
import type { TerminalSession } from "../api/terminalSessions";

const live: TerminalSession = {
  id: "a", kind: "ssh", workspaceId: "default", alias: "bastion", title: "bastion", startedAt: "2026-08-13T09:00:00Z", state: "connected", problem: "",
};
const shell: TerminalSession = {
  id: "b", kind: "shell", workspaceId: "default", title: "zsh", startedAt: "2026-08-13T09:01:00Z", state: "connected", problem: "",
};
const dead: TerminalSession = {
  id: "c", kind: "ssh", workspaceId: "default", alias: "db-primary", title: "db-primary", startedAt: "2026-08-13T09:02:00Z", state: "exited", problem: "",
  exited: { code: 255, signal: "", at: "2026-08-13T09:02:01Z" },
};

function renderList(overrides: Partial<Parameters<typeof SessionList>[0]> = {}) {
  const props = {
    sessions: [live, shell, dead],
    selected: null,
    atSessionLimit: false,
    busy: false,
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onRename: vi.fn(async () => true),
    onUnpinTitle: vi.fn(async () => true),
    onDuplicate: vi.fn(),
    onReorder: vi.fn(),
    moveTargets: [{ id: "default", label: "Main" }, { id: "w1", label: "sshc dev" }],
    onMoveToWorkspace: vi.fn(),
    heldSessionId: null,
    onHoldSession: vi.fn(),
    onMoveIn: vi.fn(),
    ...overrides,
  };
  const { rerender } = render(<SessionList {...props} />);
  return { ...props, rerender };
}

describe("SessionList", () => {
  it("offers every other workspace to move a session to", async () => {
    const user = userEvent.setup();
    const props = renderList({ sessions: [live] });
    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    const menu = screen.getByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Main" })).not.toBeInTheDocument();
    await user.click(within(menu).getByRole("menuitem", { name: "sshc dev" }));
    expect(props.onMoveToWorkspace).toHaveBeenCalledWith("a", "w1");
  });

  it("shows program progress for an unselected session and keeps connection progress separate", () => {
    const background = { ...live, programProgress: { state: "normal" as const, percent: 40 } };
    renderList({ sessions: [background, shell], selected: shell.id });

    const rows = screen.getAllByRole("listitem");
    const progress = within(rows[0]!).getByRole("progressbar", { name: "Program progress" });
    expect(progress).toHaveAttribute("aria-valuenow", "40");
    expect(progress.firstElementChild).toHaveStyle({ width: "40%" });
    expect(within(rows[1]!).queryByRole("progressbar")).toBeNull();
    expect(screen.getByRole("button", { name: "bastion" })).toHaveAccessibleDescription("connected");
  });

  it.each([
    ["error", "Program progress (error)", "bg-danger"],
    ["paused", "Program progress (paused)", "bg-notice-ink"],
  ] as const)("shows %s program progress with its accessible name and color", (state, name, color) => {
    const background = { ...live, programProgress: { state, percent: 60 } };
    renderList({ sessions: [background] });

    const progress = screen.getByRole("progressbar", { name });
    expect(progress).toHaveAttribute("aria-valuenow", "60");
    expect(progress.firstElementChild).toHaveClass(color);
  });

  it("shows indeterminate progress without claiming a percentage", () => {
    const background = { ...live, programProgress: { state: "indeterminate" as const } };
    renderList({ sessions: [background] });

    expect(screen.getByRole("progressbar", { name: "Program progress" })).not.toHaveAttribute("aria-valuenow");
  });

  it("removes the bar when the engine clears program progress", () => {
    const background = { ...live, programProgress: { state: "normal" as const, percent: 0 } };
    const { rerender, ...props } = renderList({ sessions: [background] });
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");

    rerender(<SessionList {...props} sessions={[live]} />);
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("keeps every session in one flat list, with the destination on a connected row and the state in words otherwise", () => {
    renderList();

    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(within(rows[0]!).getByText("bastion", { selector: "[data-session-destination]" })).toBeVisible();
    expect(within(rows[1]!).getByText("localhost", { selector: "[data-session-destination]" })).toBeVisible();
    expect(within(rows[2]!).getByText("exited 255", { selector: "[data-session-status-text]" })).toBeVisible();
    expect(within(rows[2]!).queryByText("db-primary", { selector: "[data-session-destination]" })).toBeNull();
    for (const [name, status] of [["bastion", "connected"], ["zsh", "connected"], ["db-primary", "exited 255"]] as const) {
      expect(screen.getByRole("button", { name })).toHaveAccessibleDescription(status);
    }
  });

  it("puts what the program says it is doing in place of the dot and the destination, until a seen result", () => {
    const { rerender, ...props } = renderList({
      sessions: [
        { ...live, programStatus: { state: "blocked", kind: "permission", message: "Run rm -rf build?", version: 2 } },
        { ...shell, programStatus: { state: "done", message: "Docs built", version: 4 } },
        { ...dead, programStatus: { state: "working", version: 1 } },
      ],
    });
    expect(screen.getByRole("button", { name: "bastion" })).toHaveAccessibleDescription("Needs permission · Run rm -rf build?");
    expect(screen.getByRole("button", { name: "bastion" }).querySelector("[data-program-status-text]")).toHaveClass("text-notice-ink");
    expect(screen.getByRole("button", { name: "zsh" })).toHaveAccessibleDescription("Done · Docs built");
    expect(screen.getByRole("button", { name: "zsh" }).querySelector("[data-program-status]")).toHaveAttribute("data-program-status", "done");
    // An ended session says how it ended, whatever its program last said.
    expect(screen.getByRole("button", { name: "db-primary" })).toHaveAccessibleDescription("exited 255");

    rerender(<SessionList {...props} seenProgramStatus={new Map([["b", 4]])} />);
    expect(screen.getByRole("button", { name: "zsh" })).toHaveAccessibleDescription("connected");
    expect(screen.getByRole("button", { name: "zsh" }).querySelector("[data-program-status]")).toBeNull();
  });

  it("shows unread terminal notifications without changing the session order", () => {
    const asking = { ...live, lastNotification: { title: "", body: "Claude needs your permission to use Bash", occurredAt: "2026-08-13T09:03:00Z" } };
    renderList({ sessions: [asking, shell, dead], unreadBySession: new Set(["b", "a"]) });

    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("bastion");
    expect(rows[1]).toHaveTextContent("zsh");
    expect(within(rows[0]!).getByText("Claude needs your permission to use Bash")).toBeVisible();
    expect(screen.getByRole("button", { name: "bastion" })).toHaveAccessibleDescription("connected · Unread notification");
    expect(screen.getByRole("button", { name: "zsh" })).toHaveAccessibleDescription("connected · Unread notification");
    expect(screen.getByRole("button", { name: "db-primary" })).toHaveAccessibleDescription("exited 255");
  });

  it("says what the program sent when the notification has a title and a body, and something when it has neither", () => {
    const titled = { ...live, lastNotification: { title: "Codex", body: "Approval requested", occurredAt: "2026-08-13T09:03:00Z" } };
    const empty = { ...shell, lastNotification: { title: "", body: "", occurredAt: "2026-08-13T09:03:00Z" } };
    renderList({ sessions: [titled, empty], unreadBySession: new Set(["a", "b"]) });

    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent(/Codex\s+Approval requested/);
    expect(rows[1]).toHaveTextContent("A program in this terminal sent a notification.");
  });

  it("names the ProxyJump hop and authentication phase while connecting", () => {
    renderList({ sessions: [{
      ...live,
      state: "connecting",
      progress: {
        phase: "authenticating", alias: "mdx-jamstec-1", hostName: "192.0.2.10",
        user: "ops", hop: 1, hops: 2,
      },
    }] });

    expect(screen.getByText("authenticating with mdx-jamstec-1 · 1/2", { selector: "[data-session-status-text]" })).toBeVisible();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("selects a session from anywhere in its main row content", async () => {
    const props = renderList();
    const detail = screen.getByText("bastion", { selector: "[data-session-destination]" });

    await userEvent.click(detail);

    expect(props.onSelect).toHaveBeenCalledWith(live.id);
    expect(detail.closest("button")).toHaveAttribute("aria-label", live.title);
  });

  it("offers taking a session out of its split only for a session in one", async () => {
    const user = userEvent.setup();
    const group = { number: 1, view: { root: { direction: "horizontal" as const, ratio: 50, first: { sessionId: "a" }, second: { sessionId: "b" } }, activeSessionId: "a", focusMode: false, focusRevision: 0 } };
    const onRemoveSessionFromSplit = vi.fn();
    renderList({ splitGroupBySession: new Map([["a", group], ["b", group]]), onRemoveSessionFromSplit });

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Remove from split view" }));
    expect(onRemoveSessionFromSplit).toHaveBeenCalledExactlyOnceWith("a");

    await user.click(screen.getByRole("button", { name: "Actions for db-primary" }));
    expect(screen.queryByRole("menuitem", { name: "Remove from split view" })).not.toBeInTheDocument();
  });

  it("renames a session in place and leaves its destination alone", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    const field = screen.getByRole("textbox", { name: "New name for bastion" });
    await user.clear(field);
    await user.type(field, "prod bastion{Enter}");

    expect(props.onRename).toHaveBeenCalledWith("a", "prod bastion");
  });

  describe.each([
    { isComposing: true },
    { isComposing: false, keyCode: 229 },
  ])("while the IME reports %j", (composition) => {
    it("keeps the name editable on composition Enter and saves on the next Enter", async () => {
      const user = userEvent.setup();
      const props = renderList();
      await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
      await user.click(screen.getByRole("menuitem", { name: "Rename" }));
      const field = screen.getByRole("textbox", { name: "New name for bastion" });
      fireEvent.compositionStart(field);
      fireEvent.change(field, { target: { value: "開発サーバ" } });
      fireEvent.keyDown(field, { key: "Enter", ...composition });

      expect(props.onRename).not.toHaveBeenCalled();
      expect(field).toHaveFocus();
      expect(field).toHaveValue("開発サーバ");

      fireEvent.compositionEnd(field);
      await user.keyboard("{Enter}");
      expect(props.onRename).toHaveBeenCalledExactlyOnceWith("a", "開発サーバ");
      expect(field).not.toBeInTheDocument();
    });

    it("keeps renaming open on composition Escape and cancels on the next Escape", async () => {
      const user = userEvent.setup();
      const props = renderList();
      await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
      await user.click(screen.getByRole("menuitem", { name: "Rename" }));
      const field = screen.getByRole("textbox", { name: "New name for bastion" });
      fireEvent.change(field, { target: { value: "開発サーバ" } });
      fireEvent.keyDown(field, { key: "Escape", ...composition });
      expect(field).toHaveFocus();
      expect(field).toHaveValue("開発サーバ");
      expect(props.onRename).not.toHaveBeenCalled();

      await user.keyboard("{Escape}");
      expect(field).not.toBeInTheDocument();
      expect(props.onRename).not.toHaveBeenCalled();
    });
  });

  it("keeps the old name when a rename is abandoned", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    await user.type(screen.getByRole("textbox", { name: "New name for bastion" }), "x{Escape}");

    expect(props.onRename).not.toHaveBeenCalled();
  });

  it("opens another session to the same place", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Duplicate session" }));

    expect(props.onDuplicate).toHaveBeenCalledWith("a");
  });

  it("reorders from the menu so a keyboard can do it too", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for zsh" }));
    await user.click(screen.getByRole("menuitem", { name: "Move up" }));

    expect(props.onReorder).toHaveBeenCalledWith(["b", "a", "c"]);
  });

  it("offers no move past either end", async () => {
    const user = userEvent.setup();
    renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));

    expect(screen.getByRole("menuitem", { name: "Move up" })).toBeDisabled();
    expect(screen.getByRole("menuitem", { name: "Move down" })).toBeEnabled();
  });

  it("closes the row menu with Escape and returns focus to its trigger", async () => {
    const user = userEvent.setup();
    renderList();
    const trigger = screen.getByRole("button", { name: "Actions for bastion" });

    await user.click(trigger);
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("opens the row menu above its button when the window has no room below", async () => {
    const user = userEvent.setup();
    renderList();
    const trigger = screen.getByRole("button", { name: "Actions for db-primary" });
    const menuHeight = 200;
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({ top: window.innerHeight - 40, bottom: window.innerHeight - 20, right: 200 } as DOMRect);
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      return (this.getAttribute("role") === "menu" ? { width: 240, height: menuHeight } : { top: 0, bottom: 0, width: 0, height: 0 }) as DOMRect;
    });

    await user.click(trigger);

    const menu = screen.getByRole("menu");
    expect(menu.parentElement).toBe(document.body);
    expect(menu.style.top).toBe(`${window.innerHeight - 40 - menuHeight - 4}px`);
  });

  it("reports selection and closing separately", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "bastion" }));
    expect(props.onSelect).toHaveBeenCalledWith("a");

    await user.click(screen.getByRole("button", { name: "Close bastion" }));
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(props.onClose).toHaveBeenCalledWith("a");
  });

  it("does not end a live connection on the first tap", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close bastion" }));

    expect(props.onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("closes immediately without a warning while Shift is held", () => {
    const props = renderList();

    fireEvent.keyDown(window, { key: "Shift" });
    fireEvent.click(screen.getByRole("button", { name: "Close bastion" }), { shiftKey: true });

    expect(props.onClose).toHaveBeenCalledWith("a");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("tints connection rows only while Shift is held and clears a missed keyup on blur", () => {
    renderList();
    const row = screen.getByRole("button", { name: "bastion" }).parentElement;

    expect(row).not.toHaveClass("bg-danger/10");
    fireEvent.keyDown(window, { key: "Shift" });
    expect(row).toHaveClass("bg-danger/10");
    fireEvent.keyUp(window, { key: "Shift" });
    expect(row).not.toHaveClass("bg-danger/10");

    fireEvent.keyDown(window, { key: "Shift" });
    fireEvent.blur(window);
    expect(row).not.toHaveClass("bg-danger/10");
  });

  it("says what ending the connection costs", async () => {
    const user = userEvent.setup();
    renderList();

    await user.click(screen.getByRole("button", { name: "Close bastion" }));

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent(/Running processes and visible output will be lost/);
    expect(dialog).toHaveTextContent(/bastion/);
  });

  it("survives a double tap on the same spot", async () => {
    const user = userEvent.setup();
    const props = renderList();

    const close = screen.getByRole("button", { name: "Close bastion" });
    await user.dblClick(close);

    expect(props.onClose).not.toHaveBeenCalled();
  });

  it("puts the keyboard on the side that loses nothing", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close bastion" }));
    await user.keyboard("{Enter}");

    expect(props.onClose).not.toHaveBeenCalled();
  });

  it("keeps the session when the question is dismissed", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close bastion" }));
    await user.click(screen.getByRole("button", { name: "Keep it open" }));

    expect(props.onClose).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes a session that already ended without asking", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close db-primary" }));

    expect(props.onClose).toHaveBeenCalledWith("c");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows nothing to open and no list when there is no session", () => {
    renderList({ sessions: [] });

    expect(screen.getByText("No session is open.")).toBeInTheDocument();
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
  });

  it("shows the forwards a session has open", () => {
    renderList({
      sessions: [{
        ...live,
        forwards: [
          { id: "pf-1", kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "", temporary: false },
          { id: "pf-2", kind: "dynamic", listen: "127.0.0.1:1080", to: "", problem: "", temporary: false },
          { id: "pf-3", kind: "agent", listen: "", to: "", problem: "", temporary: false },
        ],
      }],
    });

    expect(screen.getByText("forwarding 127.0.0.1:8080 → 10.0.0.5:80")).toBeVisible();
    expect(screen.getByText("SOCKS5 proxy on 127.0.0.1:1080")).toBeVisible();
    expect(screen.getByText("forwarding the SSH agent to the remote host")).toBeVisible();
  });

  it("says why a forward could not be opened", () => {
    renderList({
      sessions: [{
        ...live,
        forwards: [
          { id: "pf-1", kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "address_in_use", temporary: false },
        ],
      }],
    });

    expect(screen.getByText("The local port is already in use.")).toBeVisible();
    expect(screen.queryByText("address_in_use")).toBeNull();
    expect(screen.queryByText(/forwarding 127/)).toBeNull();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConsoleList } from "./ConsoleList";
import type { TerminalSession } from "../api/integrations";

const live: TerminalSession = {
  id: "a", kind: "ssh", alias: "bastion", title: "bastion", startedAt: "2026-08-13T09:00:00Z", state: "connected", problem: "",
};
const shell: TerminalSession = {
  id: "b", kind: "shell", title: "zsh", startedAt: "2026-08-13T09:01:00Z", state: "connected", problem: "",
};
const dead: TerminalSession = {
  id: "c", kind: "ssh", alias: "db-primary", title: "db-primary", startedAt: "2026-08-13T09:02:00Z", state: "exited", problem: "",
  exited: { code: 255, signal: "", at: "2026-08-13T09:02:01Z" },
};

function renderList(overrides: Partial<Parameters<typeof ConsoleList>[0]> = {}) {
  const props = {
    sessions: [live, shell, dead],
    selected: null,
    maxSessions: 50,
    busy: false,
    problem: "",
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onRename: vi.fn(async () => true),
    onUnpinTitle: vi.fn(async () => true),
    onDuplicate: vi.fn(),
    onReorder: vi.fn(),
    onOpenShell: vi.fn(),
    ...overrides,
  };
  render(<ConsoleList {...props} />);
  return props;
}

describe("ConsoleList", () => {
  it("keeps every session in one flat list and says the state in words", () => {
    renderList();

    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("connected · bastion");
    expect(rows[1]).toHaveTextContent("connected · localhost");
    expect(rows[2]).toHaveTextContent("exited 255 · db-primary");
  });

  it("shows unread Agent activity without changing the session order", () => {
    renderList({ unreadBySession: new Map([["b", "completed"], ["a", "attention"]]) });

    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("bastion");
    expect(rows[1]).toHaveTextContent("zsh");
    expect(screen.getByLabelText("Unread: input needed")).toBeVisible();
    expect(screen.getByLabelText("Unread: completed")).toBeVisible();
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

    expect(screen.getByText("authenticating with mdx-jamstec-1 · 1/2 · bastion")).toBeVisible();
  });

  it("selects a console from anywhere in its main row content", async () => {
    const props = renderList();
    const detail = screen.getByText("connected · bastion");

    await userEvent.click(detail);

    expect(props.onSelect).toHaveBeenCalledWith(live.id);
    expect(detail.closest("button")).toHaveAttribute("aria-label", live.title);
  });

  it("collapses split terminals into one live workspace entry", async () => {
    const user = userEvent.setup();
    const props = renderList({
      workspace: {
        id: "live-workspace",
        name: "bastion + db-primary",
        memberSessionIds: [live.id, dead.id],
        focusedSessionId: live.id,
      },
    });

    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByText("bastion + db-primary")).toBeVisible();
    expect(screen.getByText("2 terminals")).toBeVisible();
    expect(screen.queryByRole("button", { name: "bastion" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Show terminals in bastion + db-primary" }));
    expect(screen.getByRole("button", { name: "bastion" })).toBeVisible();
    expect(screen.getByRole("button", { name: "db-primary" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "bastion + db-primary" }));
    expect(props.onSelect).toHaveBeenCalledWith(live.id);
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

  it("keeps the old name when a rename is abandoned", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Rename" }));
    await user.type(screen.getByRole("textbox", { name: "New name for bastion" }), "x{Escape}");

    expect(props.onRename).not.toHaveBeenCalled();
  });

  it("opens another console to the same place", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Actions for bastion" }));
    await user.click(screen.getByRole("menuitem", { name: "Duplicate this connection" }));

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

  it("opens the row menu upward when the navigation has no room below", async () => {
    const user = userEvent.setup();
    renderList();
    document.body.setAttribute("data-navigation-scroll", "");
    const trigger = screen.getByRole("button", { name: "Actions for db-primary" });
    vi.spyOn(document.body, "getBoundingClientRect").mockReturnValue({ top: 0, bottom: 220 } as DOMRect);
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({ top: 180, bottom: 200 } as DOMRect);

    await user.click(trigger);

    expect(screen.getByRole("menu")).toHaveClass("bottom-full", "mb-0.5");
    document.body.removeAttribute("data-navigation-scroll");
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

  it("keeps the console when the question is dismissed", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close bastion" }));
    await user.click(screen.getByRole("button", { name: "Keep it open" }));

    expect(props.onClose).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes an console that already ended without asking", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Close db-primary" }));

    expect(props.onClose).toHaveBeenCalledWith("c");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("is the only way in to a local shell", async () => {
    const user = userEvent.setup();
    const props = renderList();

    await user.click(screen.getByRole("button", { name: "Local shell" }));

    expect(props.onOpenShell).toHaveBeenCalledOnce();
  });

  it("stops offering a new shell once the live limit is reached", () => {
    renderList({ sessions: [live, shell], maxSessions: 2 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeDisabled();
    expect(screen.getByText(/limit of 2 open consoles/)).toBeInTheDocument();
  });

  it("does not count an exited session against the limit", () => {
    renderList({ sessions: [live, dead], maxSessions: 2 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
  });

  it("does not call itself full before the limit is known", () => {
    renderList({ sessions: [], maxSessions: 0 });

    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
    expect(screen.queryByText(/limit of/)).not.toBeInTheDocument();
  });

  it("shows nothing to open and no list when there is no session", () => {
    renderList({ sessions: [] });

    expect(screen.getByText("No console is open.")).toBeInTheDocument();
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Local shell" })).toBeEnabled();
  });

  it("reports a refusal where the action was taken", () => {
    renderList({ problem: "No more consoles can be opened. Close one first." });

    expect(screen.getByRole("alert")).toHaveTextContent("No more consoles can be opened");
  });
  it("shows the forwards a console has open", () => {
    renderList({
      sessions: [{
        ...live,
        forwards: [
          { kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "" },
          { kind: "dynamic", listen: "127.0.0.1:1080", to: "", problem: "" },
          { kind: "agent", listen: "", to: "", problem: "" },
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
          { kind: "local", listen: "127.0.0.1:8080", to: "10.0.0.5:80", problem: "address already in use" },
        ],
      }],
    });

    expect(screen.getByText("address already in use")).toBeVisible();
    expect(screen.queryByText(/forwarding 127/)).toBeNull();
  });
});

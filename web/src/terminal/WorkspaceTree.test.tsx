import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import type { TerminalSessionsState } from "./sessions";
import type { TerminalWorkspacesState } from "./useTerminalWorkspaces";
import { WorkspaceTree } from "./WorkspaceTree";

const workspaces: TerminalWorkspace[] = [
  { id: "default", name: "", hidden: false, builtIn: true },
  { id: "agents", name: "", hidden: true, builtIn: true },
  { id: "w1", name: "sshc dev", hidden: false, builtIn: false },
];

const session = (id: string, workspaceId: string, extra: Partial<TerminalSession> = {}): TerminalSession => ({
  id, kind: "shell", workspaceId, title: id, startedAt: "2026-10-22T00:00:00Z", state: "connected", problem: "", ...extra,
});

function workspacesState(overrides: Partial<TerminalWorkspacesState> = {}): TerminalWorkspacesState {
  return {
    workspaces, maxWorkspaces: 50, problem: "",
    create: vi.fn(async () => null), update: vi.fn(async () => true), remove: vi.fn(async () => true),
    ...overrides,
  };
}

function sessionsState(overrides: Partial<TerminalSessionsState> = {}): TerminalSessionsState {
  return {
    sessions: [], maxSessions: 50, busy: false, problem: "", remoteRefusal: null, remoteRefusalAt: null, loaded: true,
    rename: vi.fn(async () => true), open: vi.fn(async () => null), reconnect: vi.fn(async () => true),
    stopReconnect: vi.fn(async () => true), close: vi.fn(async () => undefined), closeAll: vi.fn(async () => undefined),
    refresh: vi.fn(async () => undefined), markExited: vi.fn(), moveToWorkspace: vi.fn(async () => true),
    ...overrides,
  };
}

function renderTree(options: {
  sessions?: TerminalSession[];
  selected?: string | null;
  workspaces?: Partial<TerminalWorkspacesState>;
  terminalSessions?: Partial<TerminalSessionsState>;
  aliases?: string[];
} = {}) {
  const props = {
    workspaces: workspacesState(options.workspaces),
    terminalSessions: sessionsState(options.terminalSessions),
    sessions: options.sessions ?? [session("main-shell", "default"), session("agent-shell", "agents"), session("dev-shell", "w1")],
    selected: options.selected ?? null,
    onSelect: vi.fn(),
    onDuplicate: vi.fn(),
    onReorder: vi.fn(),
    onOpenShell: vi.fn(),
    onConnect: vi.fn(),
    aliases: options.aliases ?? [],
  };
  render(<WorkspaceTree {...props} />);
  return props;
}

afterEach(() => window.localStorage.clear());

// jsdom has no DataTransfer; this one carries what a drag sets between events.
function dragData() {
  const values = new Map<string, string>();
  return {
    get types() { return [...values.keys()]; },
    setData: (type: string, value: string) => { values.set(type, value); },
    getData: (type: string) => values.get(type) ?? "",
    effectAllowed: "",
    dropEffect: "",
  };
}

function sessionRow(name: string): HTMLElement {
  const row = screen.getByText(name).closest("li");
  if (row === null) throw new Error(`no row for ${name}`);
  return row;
}

describe("WorkspaceTree", () => {
  it.each(["remote_engine_vault_locked", "remote_engine_incompatible", "remote_engine_unreachable"])(
    "leaves the run target's %s refusal to the terminal area",
    (code) => {
      renderTree({ terminalSessions: { remoteRefusal: new ApiError(code, 409, null) } });
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /^(Unlock|Update)…$/ })).not.toBeInTheDocument();
    },
  );

  it("groups sessions by workspace and leaves the hidden agents workspace out", () => {
    renderTree();
    const tree = screen.getByRole("list", { name: "Workspaces" });
    expect(within(tree).getByRole("button", { name: /^Main/ })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: /^sshc dev/ })).toBeInTheDocument();
    expect(within(tree).queryByText("agent-shell")).not.toBeInTheDocument();
    expect(within(tree).getByText("main-shell")).toBeInTheDocument();
    expect(within(tree).getByText("dev-shell")).toBeInTheDocument();
  });

  it("shows hidden workspaces from the heading menu and remembers it", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.click(screen.getByRole("button", { name: "Workspace list actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Show hidden workspaces" }));
    expect(screen.getByText("agent-shell")).toBeInTheDocument();
    expect(window.localStorage.getItem("sshc.terminal.show-hidden-workspaces.v1")).toBe("true");
  });

  it("folds a workspace and keeps it folded", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.click(screen.getByRole("button", { name: /^sshc dev/ }));
    expect(screen.queryByText("dev-shell")).not.toBeInTheDocument();
    expect(window.localStorage.getItem("sshc.terminal.collapsed-workspaces.v1")).toBe('["w1"]');
  });

  it("opens a session in the workspace whose plus was pressed", async () => {
    const user = userEvent.setup();
    const props = renderTree({ aliases: ["bastion"] });
    await user.click(screen.getByRole("button", { name: "Open a session in sshc dev" }));
    await user.click(within(screen.getByRole("dialog", { name: "New session" })).getByRole("button", { name: /Local shell, localhost/ }));
    expect(props.onOpenShell).toHaveBeenCalledWith(undefined, "w1");

    await user.click(screen.getByRole("button", { name: "Open a session in Main" }));
    await user.click(within(screen.getByRole("dialog", { name: "New session" })).getByRole("button", { name: /bastion/ }));
    expect(props.onConnect).toHaveBeenCalledWith("bastion", "default");
  });

  it("opens the bottom new session in the selected session's workspace", async () => {
    const user = userEvent.setup();
    const props = renderTree({ selected: "dev-shell" });
    await user.click(screen.getByRole("button", { name: "New session" }));
    await user.click(within(screen.getByRole("dialog", { name: "New session" })).getByRole("button", { name: /Local shell, localhost/ }));
    expect(props.onOpenShell).toHaveBeenCalledWith(undefined, "w1");
  });

  it("stops offering a new session once the live limit is reached, not counting exited ones", () => {
    renderTree({
      sessions: [session("a", "default"), session("b", "w1")],
      terminalSessions: { maxSessions: 2 },
    });
    expect(screen.getByRole("button", { name: "New session" })).toBeDisabled();
    expect(screen.getByText(/limit of 2 open sessions/)).toBeInTheDocument();
  });

  it("does not count an exited session against the limit", () => {
    renderTree({
      sessions: [session("a", "default"), session("b", "w1", { state: "exited", exited: { code: 0, signal: "", at: "" } })],
      terminalSessions: { maxSessions: 2 },
    });
    expect(screen.getByRole("button", { name: "New session" })).toBeEnabled();
  });

  it("reports a refusal where the action was taken", () => {
    renderTree({ terminalSessions: { problem: "No more sessions can be opened. Close one first." } });
    expect(screen.getByRole("alert")).toHaveTextContent("No more sessions can be opened");
  });

  it("deletes only a workspace that is not built in, saying where its sessions go", async () => {
    const user = userEvent.setup();
    const props = renderTree();
    await user.click(screen.getByRole("button", { name: "Workspace actions for Main" }));
    expect(screen.queryByRole("menuitem", { name: "Delete workspace" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Workspace actions for sshc dev" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete workspace" }));
    const dialog = screen.getByRole("dialog", { name: "Delete sshc dev?" });
    expect(dialog).toHaveTextContent("Its 1 sessions stay open and move to Main.");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(props.workspaces.remove).toHaveBeenCalledWith("w1");
  });

  it("creates a workspace from the heading button", async () => {
    const user = userEvent.setup();
    const props = renderTree();
    await user.click(screen.getByRole("button", { name: "Workspace list actions" }));
    expect(screen.queryByRole("menuitem", { name: "New workspace" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "New workspace" }));
    await user.type(screen.getByRole("textbox", { name: "Workspace name" }), "training");
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(props.workspaces.create).toHaveBeenCalledWith("training");
  });

  it("disables workspace creation when the workspace limit is reached", () => {
    renderTree({ workspaces: { maxWorkspaces: workspaces.length } });
    expect(screen.getByRole("button", { name: "New workspace" })).toBeDisabled();
  });

  it("moves a session dropped on another workspace to the end of it", () => {
    const sessions = [session("main-shell", "default"), session("dev-shell", "w1"), session("other-shell", "default")];
    const props = renderTree({ sessions });
    const dataTransfer = dragData();
    fireEvent.dragStart(sessionRow("main-shell"), { dataTransfer });
    const target = screen.getByRole("button", { name: /^sshc dev/ }).parentElement!;
    fireEvent.dragOver(target, { dataTransfer });
    fireEvent.drop(target, { dataTransfer });

    expect(props.terminalSessions.moveToWorkspace).toHaveBeenCalledWith("main-shell", "w1");
    expect(props.onReorder).toHaveBeenCalledWith(["dev-shell", "main-shell", "other-shell"]);
  });

  it("moves a session dropped on a session of another workspace before it", () => {
    const sessions = [session("main-shell", "default"), session("dev-a", "w1"), session("dev-b", "w1")];
    const props = renderTree({ sessions });
    const dataTransfer = dragData();
    fireEvent.dragStart(sessionRow("main-shell"), { dataTransfer });
    fireEvent.dragOver(sessionRow("dev-b"), { dataTransfer });
    fireEvent.drop(sessionRow("dev-b"), { dataTransfer });

    expect(props.terminalSessions.moveToWorkspace).toHaveBeenCalledWith("main-shell", "w1");
    expect(props.onReorder).toHaveBeenCalledWith(["dev-a", "main-shell", "dev-b"]);
  });

  it("reorders a session dropped inside its own workspace without moving it", () => {
    const sessions = [session("first", "default"), session("second", "default")];
    const props = renderTree({ sessions });
    const dataTransfer = dragData();
    fireEvent.dragStart(sessionRow("second"), { dataTransfer });
    fireEvent.dragOver(sessionRow("first"), { dataTransfer });
    fireEvent.drop(sessionRow("first"), { dataTransfer });
    const ownRow = screen.getByRole("button", { name: /^Main/ }).parentElement!;
    fireEvent.drop(ownRow, { dataTransfer });

    expect(props.onReorder).toHaveBeenCalledWith(["second", "first"]);
    expect(props.terminalSessions.moveToWorkspace).not.toHaveBeenCalled();
  });
});

import { createEvent, fireEvent, render, screen, within } from "@testing-library/react";
import { currentSettings } from "../settings/settingsStore";
import { fakeSettingsServer } from "../testing/settingsServer";
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
    move: vi.fn(async () => true),
    ...overrides,
  };
}

function sessionsState(overrides: Partial<TerminalSessionsState> = {}): TerminalSessionsState {
  return {
    sessions: [], maxSessions: 50, busy: false, problem: "", remoteRefusal: null, remoteRefusalAt: null, opening: false, loaded: true, restoreOffer: undefined,
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
  unread?: ReadonlySet<string>;
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
    ...(options.unread === undefined ? {} : { unreadBySession: options.unread }),
  };
  render(<WorkspaceTree {...props} />);
  return props;
}

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

// The row a workspace is dragged by, and the item (its row and its sessions) another workspace is dropped on.
function workspaceRow(name: string): HTMLElement {
  return screen.getByRole("button", { name: new RegExp(`^${name}`) }).parentElement!;
}

function workspaceItem(name: string): HTMLElement {
  const item = workspaceRow(name).closest("li");
  if (item === null) throw new Error(`no workspace ${name}`);
  // jsdom lays nothing out; each workspace is 100px tall from the top of the page.
  item.getBoundingClientRect = () => ({ top: 0, bottom: 100, height: 100, left: 0, right: 200, width: 200, x: 0, y: 0, toJSON: () => ({}) });
  return item;
}

// jsdom has no DragEvent either, so the pointer's height is put on the event by hand.
function dragAt(kind: "dragOver" | "drop", element: HTMLElement, clientY: number, dataTransfer: ReturnType<typeof dragData>) {
  const event = createEvent[kind](element, { dataTransfer });
  Object.defineProperty(event, "clientY", { value: clientY });
  fireEvent(element, event);
}

function sessionRow(name: string): HTMLElement {
  const row = screen.getByText(name).closest("li");
  if (row === null) throw new Error(`no row for ${name}`);
  return row;
}

afterEach(() => vi.restoreAllMocks());

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

  it("counts unread notifications in the heading and opens the next one in sidebar order, going round", async () => {
    const user = userEvent.setup();
    const sessions = [session("one", "default"), session("two", "default"), session("three", "w1")];

    const first = renderTree({ sessions, unread: new Set(["three", "one"]), selected: "one" });
    await user.click(screen.getByRole("button", { name: "Unread 2" }));
    expect(first.onSelect).toHaveBeenLastCalledWith("three");
  });

  it("goes back to the first unread session after the last one", async () => {
    const user = userEvent.setup();
    const sessions = [session("one", "default"), session("two", "default"), session("three", "w1")];

    const props = renderTree({ sessions, unread: new Set(["three", "one"]), selected: "three" });
    await user.click(screen.getByRole("button", { name: "Unread 2" }));
    expect(props.onSelect).toHaveBeenLastCalledWith("one");
  });

  it("unfolds the workspace of the unread session it opens", async () => {
    const user = userEvent.setup();
    fakeSettingsServer({ state: { facts: {}, interface: { collapsedTerminalWorkspaces: '["w1"]' } } });
    renderTree({ sessions: [session("one", "default"), session("three", "w1")], unread: new Set(["three"]) });
    expect(screen.queryByText("three")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Unread 1" }));

    expect(screen.getByText("three")).toBeInTheDocument();
  });

  it("says nothing about unread notifications when there are none", () => {
    renderTree({ unread: new Set() });
    expect(screen.queryByRole("button", { name: /^Unread/ })).not.toBeInTheDocument();
  });

  it("shows hidden workspaces from the heading menu and remembers it", async () => {
    fakeSettingsServer();
    const user = userEvent.setup();
    renderTree();
    await user.click(screen.getByRole("button", { name: "Workspace list actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Show hidden workspaces" }));
    expect(screen.getByText("agent-shell")).toBeInTheDocument();
    expect(currentSettings()?.state.interface?.showHiddenTerminalWorkspaces).toBe("true");
  });

  it("folds a workspace and keeps it folded", async () => {
    fakeSettingsServer();
    const user = userEvent.setup();
    renderTree();
    await user.click(screen.getByRole("button", { name: /^sshc dev/ }));
    expect(screen.queryByText("dev-shell")).not.toBeInTheDocument();
    expect(currentSettings()?.state.interface?.collapsedTerminalWorkspaces).toBe('["w1"]');
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

  it("moves a workspace up and down its menu over the shown workspaces, stepping over hidden ones", async () => {
    const user = userEvent.setup();
    const props = renderTree();
    await user.click(screen.getByRole("button", { name: "Workspace actions for Main" }));
    expect(screen.getByRole("menuitem", { name: "Move up" })).toBeDisabled();
    await user.click(screen.getByRole("menuitem", { name: "Move down" }));
    expect(props.workspaces.move).toHaveBeenLastCalledWith("default", null);

    await user.click(screen.getByRole("button", { name: "Workspace actions for sshc dev" }));
    expect(screen.getByRole("menuitem", { name: "Move down" })).toBeDisabled();
    await user.click(screen.getByRole("menuitem", { name: "Move up" }));
    expect(props.workspaces.move).toHaveBeenLastCalledWith("w1", "default");
  });

  it("moves a workspace dropped on the upper half of another before it and on the lower half after it", () => {
    const props = renderTree();
    const dataTransfer = dragData();
    fireEvent.dragStart(workspaceRow("sshc dev"), { dataTransfer });
    dragAt("dragOver", workspaceItem("Main"), 10, dataTransfer);
    dragAt("drop", workspaceItem("Main"), 10, dataTransfer);
    expect(props.workspaces.move).toHaveBeenLastCalledWith("w1", "default");

    fireEvent.dragStart(workspaceRow("Main"), { dataTransfer });
    dragAt("drop", workspaceItem("sshc dev"), 90, dataTransfer);
    expect(props.workspaces.move).toHaveBeenLastCalledWith("default", null);
  });

  it("takes a workspace dropped on a session for the session's workspace, leaving the sessions alone", () => {
    const props = renderTree();
    const dataTransfer = dragData();
    fireEvent.dragStart(workspaceRow("sshc dev"), { dataTransfer });
    workspaceItem("Main");
    dragAt("dragOver", sessionRow("main-shell"), 10, dataTransfer);
    dragAt("drop", sessionRow("main-shell"), 10, dataTransfer);

    expect(props.workspaces.move).toHaveBeenCalledWith("w1", "default");
    expect(props.onReorder).not.toHaveBeenCalled();
    expect(props.terminalSessions.moveToWorkspace).not.toHaveBeenCalled();
  });

  it("does not move a workspace dropped on itself", () => {
    const props = renderTree();
    const dataTransfer = dragData();
    fireEvent.dragStart(workspaceRow("sshc dev"), { dataTransfer });
    dragAt("drop", workspaceItem("sshc dev"), 10, dataTransfer);
    expect(props.workspaces.move).not.toHaveBeenCalled();
  });
});

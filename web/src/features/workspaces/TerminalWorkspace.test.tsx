import { createEvent, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../../api/integrations";
import { TerminalWorkspace } from "./TerminalWorkspace";
import { liveWorkspaceStorageKey } from "./livePersistence";

beforeEach(() => window.sessionStorage.removeItem(liveWorkspaceStorageKey));

const workspace = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue([]),
  restore: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}));
const commandCenter = vi.hoisted(() => ({ targets: vi.fn() }));
vi.mock("./api", () => ({ workspaceApi: workspace }));
vi.mock("./WorkspaceCommandCenter", () => ({
  WorkspaceCommandCenter: ({ paneTargets }: { paneTargets: unknown[] }) => {
    commandCenter.targets(paneTargets);
    return <div>Command center open</div>;
  },
}));

const primary: TerminalSession = {
  id: "primary-session",
  kind: "ssh",
  alias: "edge",
  title: "Primary terminal",
  startedAt: "2026-08-24T09:00:00Z",
  state: "connected",
  problem: "",
};
const secondary: TerminalSession = {
  ...primary,
  id: "secondary-session",
  alias: "database",
  title: "Database terminal",
};
const localPrimary: TerminalSession = {
  id: "local-primary",
  kind: "shell",
  title: "zsh",
  startedAt: "2026-08-24T09:02:00Z",
  state: "connected",
  problem: "",
};
const localSecondary: TerminalSession = {
  ...localPrimary,
  id: "local-secondary",
  title: "bash",
};

function Harness() {
  const [sessions, setSessions] = useState<TerminalSession[]>([primary, secondary]);
  const [active, setActive] = useState(primary.id);
  return <TerminalWorkspace
    sessions={sessions}
    activeSessionId={active}
    onActive={setActive}
    onOpenAlias={async (alias) => {
      const duplicate: TerminalSession = {
        id: "duplicate-session",
        kind: "ssh",
        alias,
        title: "Duplicate terminal",
        startedAt: "2026-08-24T09:01:00Z",
        state: "connected",
        problem: "",
      };
      setSessions((current) => current.some((session) => session.id === duplicate.id) ? current : [...current, duplicate]);
      return duplicate;
    }}
    onOpenShell={vi.fn()}
    renderTerminal={(session) => <div>{session.title}</div>}
  />;
}

function paneTitles(container: HTMLElement): string[] {
  return [...container.querySelectorAll<HTMLElement>("[data-workspace-pane]")].map((pane) => pane.textContent ?? "");
}

function dragAt(target: HTMLElement, kind: "dragEnter" | "drop", dataTransfer: object, clientX: number, clientY: number) {
  const event = createEvent[kind](target, { dataTransfer });
  Object.defineProperties(event, {
    clientX: { value: clientX },
    clientY: { value: clientY },
  });
  fireEvent(target, event);
}

function consoleTransfer(sessionId: string) {
  return {
    effectAllowed: "none",
    dropEffect: "none",
    types: ["application/x-sshc-console"],
    setData: vi.fn(),
    getData: (kind: string) => kind === "application/x-sshc-console" ? sessionId : "",
  };
}

function dockConnectedSession(container: HTMLElement, sessionId = secondary.id) {
  const target = container.querySelector<HTMLElement>("[data-single-terminal-drop-target]");
  expect(target).not.toBeNull();
  Object.defineProperty(target, "getBoundingClientRect", { value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
  dragAt(target as HTMLElement, "drop", consoleTransfer(sessionId), 95, 50);
}

describe("TerminalWorkspace pane movement", () => {
  it("reattaches a live split with its ratio and focused pane after remount", async () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 65,
          first: { pane: { id: "pane-primary", sessionId: primary.id } },
          second: { pane: { id: "pane-secondary", sessionId: secondary.id } },
        },
      },
      focusedPaneId: "pane-secondary",
      focusModePaneId: null,
      name: "Build workers",
    }));
    const active = vi.fn();

    const { container } = render(<TerminalWorkspace
      sessions={[primary, secondary]}
      sessionsLoaded
      activeSessionId={primary.id}
      onActive={active}
      onOpenAlias={vi.fn()}
      onOpenShell={vi.fn()}
      renderTerminal={(session) => <div>{session.title}</div>}
    />);

    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(paneTitles(container)).toEqual(expect.arrayContaining([
      expect.stringContaining("Primary terminal"),
      expect.stringContaining("Database terminal"),
    ]));
    const separator = screen.getByRole("separator");
    expect(separator.previousElementSibling).toHaveStyle({ flexBasis: "65%" });
    expect(screen.getByText("Build workers")).toBeVisible();
    expect(active).toHaveBeenCalledWith(secondary.id);
  });

  it("waits for the engine session listing before reconciling the live snapshot", async () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "pane-primary", sessionId: primary.id } },
          second: { pane: { id: "pane-secondary", sessionId: secondary.id } },
        },
      },
      focusedPaneId: "pane-secondary",
      focusModePaneId: null,
    }));
    const active = vi.fn();
    const properties = {
      sessions: [primary, secondary],
      activeSessionId: primary.id,
      onActive: active,
      onOpenAlias: vi.fn(),
      onOpenShell: vi.fn(),
      renderTerminal: (session: TerminalSession) => <div>{session.title}</div>,
    };

    const { container, rerender } = render(<TerminalWorkspace {...properties} sessionsLoaded={false} />);
    expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(0);
    expect(screen.getByText("Primary terminal")).toBeVisible();
    expect(window.sessionStorage.getItem(liveWorkspaceStorageKey)).not.toBeNull();

    rerender(<TerminalWorkspace {...properties} sessionsLoaded />);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(active).toHaveBeenCalledWith(secondary.id);
  });

  it("restores Focus Mode for a surviving pane", async () => {
    window.sessionStorage.setItem(liveWorkspaceStorageKey, JSON.stringify({
      version: 1,
      root: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "pane-primary", sessionId: primary.id } },
          second: { pane: { id: "pane-secondary", sessionId: secondary.id } },
        },
      },
      focusedPaneId: "pane-secondary",
      focusModePaneId: "pane-secondary",
    }));

    const { container } = render(<TerminalWorkspace
      sessions={[primary, secondary]}
      sessionsLoaded
      activeSessionId={secondary.id}
      onActive={() => undefined}
      onOpenAlias={vi.fn()}
      onOpenShell={vi.fn()}
      renderTerminal={(session) => <div>{session.title}</div>}
    />);

    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(1));
    expect(screen.getByText("Database terminal")).toBeVisible();
    expect(screen.queryByText("Primary terminal")).toBeNull();
    expect(screen.getAllByRole("button", { name: /Exit focus mode/ }).length).toBeGreaterThan(0);
  });

  it("shows the sshc command name when no console is open", async () => {
    const { container } = render(
      <TerminalWorkspace
        sessions={[]}
        activeSessionId={null}
        onActive={() => undefined}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={() => null}
      />,
    );

    expect(await screen.findByRole("status")).toHaveTextContent("sshc host");
    expect(screen.getByRole("status")).not.toHaveTextContent("ssh host");
    expect(container.querySelector("[data-desktop-workspace-controls]")).toHaveClass("hidden", "md:flex");
    expect(screen.queryByRole("button", { name: "Split right" })).toBeNull();
  });

  it("does not show workspace management for one terminal", () => {
    const { container } = render(
      <TerminalWorkspace
        sessions={[localPrimary]}
        activeSessionId={localPrimary.id}
        onActive={() => undefined}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />,
    );

    expect(screen.getByText("zsh")).toBeVisible();
    expect(container.querySelector("[data-desktop-workspace-controls]")).toBeNull();
    expect(screen.queryByText("1 terminal")).toBeNull();
    expect(screen.queryByRole("button", { name: "Send command…" })).toBeNull();
    expect(screen.queryByText("Saved layouts")).toBeNull();
  });

  it("swaps panes by drag and drop and exposes the same operation to keyboard users", async () => {
    const user = userEvent.setup();
    const { container } = render(<Harness />);
    dockConnectedSession(container);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(paneTitles(container)[0]).toContain("Primary terminal");
    expect(paneTitles(container)[1]).toContain("Database terminal");

    let handles = screen.getAllByRole("button", { name: /Move .* pane/ });
    const target = handles[1]?.closest<HTMLElement>("[data-workspace-pane]");
    expect(target).not.toBeNull();
    const values = new Map<string, string>();
    const dataTransfer = {
      effectAllowed: "none",
      dropEffect: "none",
      types: ["application/x-sshc-console"],
      setData: (kind: string, value: string) => values.set(kind, value),
      getData: (kind: string) => values.get(kind) ?? "",
    };
    Object.defineProperty(target, "getBoundingClientRect", { value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
    fireEvent.dragStart(handles[0] as HTMLElement, { dataTransfer });
    dragAt(target as HTMLElement, "dragEnter", dataTransfer, 95, 50);
    expect(target).toHaveTextContent("Place on the right");
    dragAt(target as HTMLElement, "drop", dataTransfer, 95, 50);
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Database terminal"));
    expect(paneTitles(container)[1]).toContain("Primary terminal");

    handles = screen.getAllByRole("button", { name: /Move .* pane/ });
    await user.click(handles[0] as HTMLElement);
    expect(handles[0]).toHaveAttribute("aria-pressed", "true");
    await user.click(handles[1] as HTMLElement);
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Primary terminal"));
    expect(paneTitles(container)[1]).toContain("Database terminal");
  });

  it("creates a live workspace by docking an already connected terminal on a pane edge", async () => {
    const changed = vi.fn();
    function DockHarness() {
      const [active, setActive] = useState(primary.id);
      return <TerminalWorkspace
        sessions={[primary, secondary]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        onLiveWorkspaceChange={changed}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }
    const { container } = render(<DockHarness />);
    const target = container.querySelector<HTMLElement>("[data-single-terminal-drop-target]");
    expect(target).not.toBeNull();
    Object.defineProperty(target, "getBoundingClientRect", { value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
    const dataTransfer = {
      effectAllowed: "none",
      dropEffect: "none",
      types: ["application/x-sshc-console"],
      setData: vi.fn(),
      getData: (kind: string) => kind === "application/x-sshc-console" ? secondary.id : "",
    };

    dragAt(target as HTMLElement, "dragEnter", dataTransfer, 95, 50);
    expect(container.querySelector("[data-dock-preview='right']")).toHaveTextContent("Place on the right");
    dragAt(target as HTMLElement, "drop", dataTransfer, 95, 50);

    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(paneTitles(container)[0]).toContain("Primary terminal");
    expect(paneTitles(container)[1]).toContain("Database terminal");
    await waitFor(() => expect(changed).toHaveBeenCalledWith(expect.objectContaining({
      name: "edge + database",
      memberSessionIds: [primary.id, secondary.id],
      focusedSessionId: secondary.id,
    })));
  });

  it("renames an unsaved live workspace and publishes the new name", async () => {
    const changed = vi.fn();
    const prompt = vi.spyOn(window, "prompt").mockReturnValue("Build workers");
    function LocalHarness() {
      const [active, setActive] = useState(localPrimary.id);
      return <TerminalWorkspace
        sessions={[localPrimary, localSecondary]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        onLiveWorkspaceChange={changed}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }

    const { container } = render(<LocalHarness />);
    dockConnectedSession(container, localSecondary.id);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(screen.getByText("localhost")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Rename workspace" }));

    expect(prompt).toHaveBeenCalledWith("Live workspace name", "localhost");
    expect(screen.getByText("Build workers")).toBeVisible();
    await waitFor(() => expect(changed).toHaveBeenCalledWith(expect.objectContaining({
      name: "Build workers",
      memberSessionIds: [localPrimary.id, localSecondary.id],
    })));
    await waitFor(() => expect(JSON.parse(window.sessionStorage.getItem(liveWorkspaceStorageKey) ?? "{}")).toEqual(expect.objectContaining({
      name: "Build workers",
    })));
  });

  it("splits local shells and saves them as restorable workspace panes", async () => {
    workspace.create.mockResolvedValue({
      id: "local-workspace",
      name: "Local pair",
      layout: { pane: { id: "saved", alias: "localhost", kind: "shell" } },
      focusedPaneId: "saved",
      createdAt: "2026-08-24T10:00:00Z",
      updatedAt: "2026-08-24T10:00:00Z",
    });
    workspace.list.mockResolvedValueOnce([]).mockResolvedValueOnce([]);
    vi.spyOn(window, "prompt").mockReturnValue("Local pair");

    function LocalHarness() {
      const [active, setActive] = useState(localPrimary.id);
      return <TerminalWorkspace
        sessions={[localPrimary, localSecondary]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }

    const { container } = render(<LocalHarness />);
    dockConnectedSession(container, localSecondary.id);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(screen.getAllByText("zsh")).not.toHaveLength(0);
    expect(screen.getAllByText("bash")).not.toHaveLength(0);

    await userEvent.click(screen.getByText("Saved layouts", { selector: "summary" }));
    await userEvent.click(screen.getByRole("button", { name: "Save with a name" }));
    await waitFor(() => expect(workspace.create).toHaveBeenCalledWith(expect.objectContaining({
      name: "Local pair",
      layout: expect.objectContaining({
        split: expect.objectContaining({
          first: { pane: expect.objectContaining({ alias: "localhost", kind: "shell" }) },
          second: { pane: expect.objectContaining({ alias: "localhost", kind: "shell" }) },
        }),
      }),
    })));
  });

  it("rejects a fifth terminal while keeping the four-pane layout", async () => {
    const extra = [
      { ...primary, id: "logs-session", alias: "logs", title: "Logs terminal" },
      { ...primary, id: "metrics-session", alias: "metrics", title: "Metrics terminal" },
      { ...primary, id: "worker-session", alias: "worker", title: "Worker terminal" },
    ];
    function LimitHarness() {
      const [active, setActive] = useState(primary.id);
      return <TerminalWorkspace
        sessions={[primary, secondary, ...extra]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }
    const { container } = render(<LimitHarness />);
    dockConnectedSession(container);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));

    for (const session of extra) {
      const target = container.querySelector<HTMLElement>("[data-workspace-pane]");
      expect(target).not.toBeNull();
      Object.defineProperty(target, "getBoundingClientRect", { configurable: true, value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
      dragAt(target as HTMLElement, "drop", consoleTransfer(session.id), 95, 50);
      if (session.id !== "worker-session") {
        await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(session.id === "logs-session" ? 3 : 4));
      }
    }

    expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(4);
    expect(screen.getByRole("alert")).toHaveTextContent("up to 4 terminals");
    expect(screen.queryByText("Worker terminal")).toBeNull();
  });

  it("shows one workspace terminal at a time on a compact viewport", async () => {
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query === "(max-width: 767px)",
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia;
    try {
      function CompactHarness() {
        const [active, setActive] = useState(primary.id);
        return <TerminalWorkspace
          sessions={[primary, secondary]}
          activeSessionId={active}
          onActive={setActive}
          onOpenAlias={vi.fn()}
          onOpenShell={vi.fn()}
          renderTerminal={(session) => <div>{session.title}</div>}
        />;
      }
      const user = userEvent.setup();
      const { container } = render(<CompactHarness />);
      const target = container.querySelector<HTMLElement>("[data-single-terminal-drop-target]");
      expect(target).not.toBeNull();
      Object.defineProperty(target, "getBoundingClientRect", { value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
      const dataTransfer = {
        effectAllowed: "none",
        dropEffect: "none",
        types: ["application/x-sshc-console"],
        setData: vi.fn(),
        getData: (kind: string) => kind === "application/x-sshc-console" ? secondary.id : "",
      };
      dragAt(target as HTMLElement, "drop", dataTransfer, 95, 50);

      await waitFor(() => expect(screen.getByRole("navigation", { name: "Workspace terminals" })).toBeVisible());
      expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(1);
      expect(container.querySelector("[data-pane-toolbar]")).toBeNull();
      expect(container.querySelector("[data-desktop-workspace-controls]")).toHaveClass("hidden", "md:flex");
      expect(screen.getByText("Database terminal")).toBeVisible();
      expect(screen.queryByText("Primary terminal")).toBeNull();

      await user.click(screen.getByRole("button", { name: "edge" }));
      await waitFor(() => expect(screen.getByText("Primary terminal")).toBeVisible());
      expect(screen.queryByText("Database terminal")).toBeNull();
    } finally {
      window.matchMedia = originalMatchMedia;
    }
  });

  it("keeps a workspace through a transiently incomplete session refresh", async () => {
    function ReconciliationHarness({ available }: { available: TerminalSession[] }) {
      const [active, setActive] = useState(primary.id);
      return <TerminalWorkspace
        sessions={available}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }
    const { container, rerender } = render(<ReconciliationHarness available={[primary, secondary]} />);
    const target = container.querySelector<HTMLElement>("[data-single-terminal-drop-target]");
    expect(target).not.toBeNull();
    Object.defineProperty(target, "getBoundingClientRect", { value: () => ({ left: 0, right: 100, top: 0, bottom: 100, width: 100, height: 100 }) });
    const dataTransfer = {
      effectAllowed: "none",
      dropEffect: "none",
      types: ["application/x-sshc-console"],
      setData: vi.fn(),
      getData: (kind: string) => kind === "application/x-sshc-console" ? secondary.id : "",
    };
    dragAt(target as HTMLElement, "drop", dataTransfer, 95, 50);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));

    rerender(<ReconciliationHarness available={[primary]} />);
    rerender(<ReconciliationHarness available={[primary, secondary]} />);
    await new Promise((resolve) => window.setTimeout(resolve, 550));

    expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2);
  });

  it("resizes a split with the keyboard and can focus one pane", async () => {
    const user = userEvent.setup();
    const { container } = render(<Harness />);
    dockConnectedSession(container);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));

    const separator = screen.getByRole("separator", { name: "Resize split" });
    expect(separator).toHaveAttribute("aria-valuenow", "50");
    fireEvent.keyDown(separator, { key: "ArrowRight" });
    expect(separator).toHaveAttribute("aria-valuenow", "55");

    const focusButtons = screen.getAllByRole("button", { name: "Focus edge" });
    await user.click(focusButtons[0] as HTMLElement);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(1));
    expect(screen.getAllByRole("button", { name: "Exit focus mode" })).toHaveLength(2);
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
  });

  it("targets every connected live session even when duplicate aliases are focused away", async () => {
    commandCenter.targets.mockClear();
    const duplicate: TerminalSession = { ...secondary, alias: "edge", title: "Second edge terminal" };
    function CommandHarness() {
      const [active, setActive] = useState(primary.id);
      return <TerminalWorkspace
        sessions={[primary, duplicate]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }
    const user = userEvent.setup();
    const { container } = render(<CommandHarness />);
    dockConnectedSession(container, duplicate.id);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));

    await user.click(screen.getAllByRole("button", { name: "Focus edge" })[0] as HTMLElement);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(1));
    await user.click(screen.getByRole("button", { name: "Send command…" }));

    expect(commandCenter.targets).toHaveBeenCalledWith([
      {
        targetId: expect.any(String), sessionId: primary.id, alias: "edge", title: "Primary terminal",
        paneNumber: 1, connected: true, state: "connected",
      },
      {
        targetId: expect.any(String), sessionId: duplicate.id, alias: "edge", title: "Second edge terminal",
        paneNumber: 2, connected: true, state: "connected",
      },
    ]);
  });

  it("targets local and SSH panes through the same command center", async () => {
    commandCenter.targets.mockClear();
    function MixedCommandHarness() {
      const [active, setActive] = useState(primary.id);
      return <TerminalWorkspace
        sessions={[primary, localPrimary]}
        activeSessionId={active}
        onActive={setActive}
        onOpenAlias={vi.fn()}
        onOpenShell={vi.fn()}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }
    const user = userEvent.setup();
    const { container } = render(<MixedCommandHarness />);
    dockConnectedSession(container, localPrimary.id);
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));

    await user.click(screen.getByRole("button", { name: "Send command…" }));
    expect(commandCenter.targets).toHaveBeenCalledWith([
      {
        targetId: expect.any(String), sessionId: primary.id, alias: "edge", title: "Primary terminal",
        paneNumber: 1, connected: true, state: "connected",
      },
      {
        targetId: expect.any(String), sessionId: localPrimary.id, alias: "localhost", title: "zsh",
        paneNumber: 2, connected: true, state: "connected",
      },
    ]);
  });

  it("does not show workspace command delivery for one reconnecting session", () => {
    render(<TerminalWorkspace
      sessions={[{ ...primary, state: "reconnecting" }]}
      activeSessionId={primary.id}
      onActive={() => undefined}
      onOpenAlias={vi.fn()}
      onOpenShell={vi.fn()}
      renderTerminal={() => null}
    />);

    expect(screen.queryByRole("button", { name: "Send command…" })).toBeNull();
  });

  it("consumes a Home restore request once and leaves only the failed pane unavailable", async () => {
    workspace.restore.mockResolvedValue({
      id: "workspace-1",
      name: "Production pair",
      layout: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "pane-a", alias: "web-a" } },
          second: { pane: { id: "pane-b", alias: "web-b" } },
        },
      },
      focusedPaneId: "pane-a",
      createdAt: "2026-08-24T10:00:00Z",
      updatedAt: "2026-08-24T11:00:00Z",
    });
    const open = vi.fn(async (alias: string) => alias === "web-a"
      ? { ...primary, id: "restored-a", alias: "web-a", title: "Restored web-a" }
      : null);

    function RestoreHarness({ sequence }: { sequence: number }) {
      const [sessions, setSessions] = useState<TerminalSession[]>([]);
      return <TerminalWorkspace
        sessions={sessions}
        activeSessionId={null}
        onActive={() => undefined}
        onOpenAlias={async (alias) => {
          const session = await open(alias);
          if (session !== null) setSessions((current) => [...current, session]);
          return session;
        }}
        onOpenShell={vi.fn()}
        restoreRequest={{ id: "workspace-1", sequence }}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }

    const { container, rerender } = render(<RestoreHarness sequence={1} />);
    await waitFor(() => expect(open).toHaveBeenCalledTimes(2));
    expect(workspace.restore).toHaveBeenCalledTimes(1);
    expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2);
    expect(screen.getByText("Restored web-a")).toBeInTheDocument();
    expect(screen.getByText("The console could not be opened.")).toBeInTheDocument();
    expect(screen.queryByText("open_failed")).not.toBeInTheDocument();

    rerender(<RestoreHarness sequence={1} />);
    await new Promise((resolve) => window.setTimeout(resolve, 0));
    expect(workspace.restore).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledTimes(2);
  });

  it("recreates local panes through the local shell entry point", async () => {
    workspace.restore.mockResolvedValue({
      id: "local-workspace",
      name: "Local pair",
      layout: {
        split: {
          direction: "horizontal",
          ratio: 50,
          first: { pane: { id: "local-a", alias: "localhost", kind: "shell" } },
          second: { pane: { id: "local-b", alias: "localhost", kind: "shell" } },
        },
      },
      focusedPaneId: "local-a",
      createdAt: "2026-08-24T10:00:00Z",
      updatedAt: "2026-08-24T11:00:00Z",
    });
    const onOpenAlias = vi.fn();
    const opened = [localPrimary, localSecondary];
    const onOpenShell = vi.fn(async () => opened.shift() ?? null);

    render(<TerminalWorkspace
      sessions={[]}
      activeSessionId={null}
      onActive={vi.fn()}
      onOpenAlias={onOpenAlias}
      onOpenShell={onOpenShell}
      restoreRequest={{ id: "local-workspace", sequence: 1 }}
      renderTerminal={() => null}
    />);

    await waitFor(() => expect(onOpenShell).toHaveBeenCalledTimes(2));
    expect(onOpenAlias).not.toHaveBeenCalled();
  });
});

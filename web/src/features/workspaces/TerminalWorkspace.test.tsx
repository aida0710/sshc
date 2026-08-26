import { createEvent, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../../api/integrations";
import { TerminalWorkspace } from "./TerminalWorkspace";

const workspace = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue([]),
  restore: vi.fn(),
}));
vi.mock("./api", () => ({ workspaceApi: workspace }));

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

function Harness() {
  const [sessions, setSessions] = useState<TerminalSession[]>([primary]);
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

describe("TerminalWorkspace pane movement", () => {
  it("shows the sshc command name when no console is open", async () => {
    const { container } = render(
      <TerminalWorkspace
        sessions={[]}
        activeSessionId={null}
        onActive={() => undefined}
        onOpenAlias={vi.fn()}
        renderTerminal={() => null}
      />,
    );

    expect(await screen.findByRole("status")).toHaveTextContent("sshc host");
    expect(screen.getByRole("status")).not.toHaveTextContent("ssh host");
    expect(container.querySelector("[data-desktop-workspace-controls]")).toHaveClass("hidden", "md:flex");
  });

  it("swaps panes by drag and drop and exposes the same operation to keyboard users", async () => {
    const user = userEvent.setup();
    const { container } = render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Split right" }));
    await waitFor(() => expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2));
    expect(paneTitles(container)[0]).toContain("Primary terminal");
    expect(paneTitles(container)[1]).toContain("Duplicate terminal");

    let handles = screen.getAllByRole("button", { name: /Move edge pane/ });
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
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Duplicate terminal"));
    expect(paneTitles(container)[1]).toContain("Primary terminal");

    handles = screen.getAllByRole("button", { name: /Move edge pane/ });
    await user.click(handles[0] as HTMLElement);
    expect(handles[0]).toHaveAttribute("aria-pressed", "true");
    await user.click(handles[1] as HTMLElement);
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Primary terminal"));
    expect(paneTitles(container)[1]).toContain("Duplicate terminal");
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
    await user.click(screen.getByRole("button", { name: "Split right" }));
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
        restoreRequest={{ id: "workspace-1", sequence }}
        renderTerminal={(session) => <div>{session.title}</div>}
      />;
    }

    const { container, rerender } = render(<RestoreHarness sequence={1} />);
    await waitFor(() => expect(open).toHaveBeenCalledTimes(2));
    expect(workspace.restore).toHaveBeenCalledTimes(1);
    expect(container.querySelectorAll("[data-workspace-pane]")).toHaveLength(2);
    expect(screen.getByText("Restored web-a")).toBeInTheDocument();
    expect(screen.getByText("open_failed")).toBeInTheDocument();

    rerender(<RestoreHarness sequence={1} />);
    await new Promise((resolve) => window.setTimeout(resolve, 0));
    expect(workspace.restore).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledTimes(2);
  });
});

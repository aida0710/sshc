import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../../api/integrations";
import { TerminalWorkspace } from "./TerminalWorkspace";

const workspace = vi.hoisted(() => ({ list: vi.fn().mockResolvedValue([]) }));
vi.mock("./api", () => ({ workspaceApi: workspace }));

const primary: TerminalSession = {
  id: "primary-session",
  kind: "ssh",
  alias: "edge",
  title: "Primary terminal",
  startedAt: "2026-08-24T09:00:00Z",
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

describe("TerminalWorkspace pane movement", () => {
  it("shows the sshc command name when no console is open", async () => {
    render(
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
      setData: (kind: string, value: string) => values.set(kind, value),
      getData: (kind: string) => values.get(kind) ?? "",
    };
    fireEvent.dragStart(handles[0] as HTMLElement, { dataTransfer });
    fireEvent.dragEnter(target as HTMLElement, { dataTransfer });
    expect(target).toHaveTextContent("Exchange with edge");
    fireEvent.drop(target as HTMLElement, { dataTransfer });
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Duplicate terminal"));
    expect(paneTitles(container)[1]).toContain("Primary terminal");

    handles = screen.getAllByRole("button", { name: /Move edge pane/ });
    await user.click(handles[0] as HTMLElement);
    expect(handles[0]).toHaveAttribute("aria-pressed", "true");
    await user.click(handles[1] as HTMLElement);
    await waitFor(() => expect(paneTitles(container)[0]).toContain("Primary terminal"));
    expect(paneTitles(container)[1]).toContain("Duplicate terminal");
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
});

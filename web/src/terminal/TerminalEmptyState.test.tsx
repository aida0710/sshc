import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectionHistory, MachineStatus } from "../api/terminalOverview";
import { fakeSettingsServer } from "../testing/settingsServer";
import { TerminalEmptyState } from "./TerminalEmptyState";

const overview = vi.hoisted(() => ({
  runTargets: [] as (string | null)[],
  machineReads: 0,
  machine: null as MachineStatus | null,
  history: null as ConnectionHistory | null,
}));

vi.mock("../api/terminalOverview", () => ({
  terminalOverviewApi: (runTarget: string | null) => {
    overview.runTargets.push(runTarget);
    return {
      machineStatus: () => {
        overview.machineReads++;
        return overview.machine === null ? Promise.reject(new Error("request_failed")) : Promise.resolve(overview.machine);
      },
      connectionHistory: () => (overview.history === null ? Promise.reject(new Error("request_failed")) : Promise.resolve(overview.history)),
    };
  },
}));

// sync.configured is what this machine's sync status says; null makes the status fail to read.
const sync = vi.hoisted(() => ({ configured: false as boolean | null, reads: 0 }));

vi.mock("../api/sync", () => ({
  syncApi: {
    syncStatus: () => {
      sync.reads++;
      return sync.configured === null ? Promise.reject(new Error("request_failed")) : Promise.resolve({ configured: sync.configured });
    },
  },
}));

const noHistory: ConnectionHistory = { hours: [], recent: [] };

beforeEach(() => {
  overview.runTargets = [];
  overview.machineReads = 0;
  overview.machine = null;
  overview.history = noHistory;
  sync.configured = false;
  sync.reads = 0;
});

function renderEmptyState(runTarget: string | null, visible = true) {
  const handlers = {
    onNewSession: vi.fn(), onAddConnection: vi.fn(), onOpenSync: vi.fn(), onOpenConnection: vi.fn(), onShowConnections: vi.fn(),
  };
  const rendered = render(<TerminalEmptyState runTarget={runTarget} visible={visible} {...handlers} />);
  const showAgain = (shown: boolean) => rendered.rerender(<TerminalEmptyState runTarget={runTarget} visible={shown} {...handlers} />);
  return { handlers, container: rendered.container, showAgain };
}

describe("TerminalEmptyState", () => {
  it("opens the new session picker, adds a connection or goes to Sync from its tiles", async () => {
    const { handlers } = renderEmptyState(null);

    await userEvent.click(screen.getByRole("button", { name: /^Open a session/ }));
    expect(handlers.onNewSession).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: /^Add connection/ }));
    expect(handlers.onAddConnection).toHaveBeenCalledOnce();
    await userEvent.click(await screen.findByRole("button", { name: /^Receive by sync/ }));
    expect(handlers.onOpenSync).toHaveBeenCalledOnce();
  });

  it("says the session opens on this machine when no run target is chosen", () => {
    renderEmptyState(null);
    expect(screen.getByText("Choose a connection or a shell on this machine")).toBeInTheDocument();
  });

  it("leaves the way to Sync out once sync is set up", async () => {
    sync.configured = true;
    renderEmptyState(null);

    await waitFor(() => expect(sync.reads).toBe(1));
    expect(screen.getByRole("button", { name: /^Open a session/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Receive by sync/ })).toBeNull();
  });

  it("keeps the way to Sync when the sync status cannot be read", async () => {
    sync.configured = null;
    renderEmptyState(null);
    expect(await screen.findByRole("button", { name: /^Receive by sync/ })).toBeInTheDocument();
  });

  it("reads sync again when the terminal is shown again, and not while it is hidden", async () => {
    const { showAgain } = renderEmptyState(null);
    expect(await screen.findByRole("button", { name: /^Receive by sync/ })).toBeInTheDocument();

    sync.configured = true;
    showAgain(false);
    expect(sync.reads).toBe(1);
    showAgain(true);
    await waitFor(() => expect(screen.queryByRole("button", { name: /^Receive by sync/ })).toBeNull());
    expect(sync.reads).toBe(2);
  });

  it("names the run target a new shell would open on, and reads that engine's machine and connections", async () => {
    overview.machine = { hostname: "office", os: "windows" };
    renderEmptyState("office-pc");

    expect(screen.getByText("Choose a connection or a shell to open on office-pc")).toBeInTheDocument();
    expect(overview.runTargets).toContain("office-pc");
    expect(await screen.findByText("office · Windows")).toBeInTheDocument();
    expect(screen.getByLabelText("The machine new shells open on")).toHaveTextContent(/^office-pc/);
  });

  it("shows what the machine gives and leaves out what it does not", async () => {
    overview.machine = { hostname: "workstation", os: "darwin", uptimeSeconds: 3 * 24 * 60 * 60 + 5, memoryTotalBytes: 16 * 1024 ** 3 };
    renderEmptyState(null);

    const machine = await screen.findByLabelText("The machine new shells open on");
    expect(machine).toHaveTextContent(/^workstation · macOS/);
    expect(machine).toHaveTextContent("Memory 16.0 GiB");
    expect(machine).toHaveTextContent("Up 3d");
    expect(machine).not.toHaveTextContent("CPU");
  });

  it("does not read the machine while another page is shown over the terminal", async () => {
    overview.machine = { hostname: "workstation", os: "linux" };
    renderEmptyState(null, false);

    await waitFor(() => expect(overview.runTargets.length).toBeGreaterThan(0));
    expect(overview.machineReads).toBe(0);
    expect(screen.queryByLabelText("The machine new shells open on")).toBeNull();
  });

  it("leaves the machine out when the engine cannot tell it", async () => {
    overview.machine = null;
    renderEmptyState(null);

    await waitFor(() => expect(overview.runTargets.length).toBeGreaterThan(0));
    expect(screen.queryByLabelText("The machine new shells open on")).toBeNull();
  });

  it("lists the connections used last and opens one", async () => {
    overview.history = {
      hours: [{ hour: new Date().toISOString(), connections: 2, aliases: ["web-01", "db"] }],
      recent: [
        { alias: "web-01", hostName: "127.0.0.1", user: "deploy", port: "22", lastConnectedAt: new Date().toISOString() },
        { alias: "db", hostName: "198.51.100.20", user: "postgres", port: "22", lastConnectedAt: new Date().toISOString() },
      ],
    };
    const { handlers } = renderEmptyState(null);

    const recent = await screen.findByRole("region", { name: "Recent connections" });
    await userEvent.click(within(recent).getByRole("button", { name: /web-01/ }));
    expect(handlers.onOpenConnection).toHaveBeenCalledWith("web-01");
    await userEvent.click(within(recent).getByRole("button", { name: "Show all" }));
    expect(handlers.onShowConnections).toHaveBeenCalledOnce();
    expect(screen.getByRole("region", { name: "Activity" })).toHaveTextContent("Connections: 2 · Hosts: 2");
  });

  it("leaves the activity and the recent connections out until a connection has been opened", async () => {
    overview.history = noHistory;
    renderEmptyState(null);

    await waitFor(() => expect(overview.runTargets.length).toBeGreaterThan(0));
    expect(screen.queryByRole("region", { name: "Activity" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Recent connections" })).toBeNull();
  });

  it("draws the network behind the tiles until another art is chosen", () => {
    fakeSettingsServer();
    const { container } = renderEmptyState(null);
    expect(container.querySelector("canvas")).toHaveAttribute("aria-hidden", "true");
  });

  it("draws no art when the preset chooses none", () => {
    fakeSettingsServer({ presets: [{ id: "default", appearance: { emptyTerminalArt: "none" } }] });
    const { container } = renderEmptyState(null);
    expect(container.querySelector("canvas")).toBeNull();
  });
});

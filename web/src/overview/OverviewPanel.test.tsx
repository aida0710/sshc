import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Overview } from "../api/config";
import type { SyncStatus } from "../api/sync";
import { formatDateTime } from "../ui/format";
import { OverviewPanel } from "./OverviewPanel";

const overview = {
  entry: { absolute: "/Users/test/.ssh/config", path: "config" },
  files: [],
  hosts: [
    {
      identity: { path: "connections/work.conf", alias: "database" },
      file: { absolute: "/Users/test/.ssh/connections/work.conf", path: "connections/work.conf" },
      line: 1,
      patterns: ["database"],
      editable: true,
      group: "work",
      hostName: "db.example.com",
      user: "deploy",
      port: "2202",
    },
    {
      identity: { path: "config", alias: "bastion" },
      file: { absolute: "/Users/test/.ssh/config", path: "config" },
      line: 4,
      patterns: ["bastion"],
      editable: true,
      hostName: "203.0.113.10",
      user: "ops",
      port: "22",
    },
    {
      identity: { path: "config", alias: "" },
      file: { absolute: "/Users/test/.ssh/config", path: "config" },
      line: 9,
      patterns: ["*.internal"],
      editable: true,
    },
  ],
  groups: [
    { name: "work", directory: "connections/work", keyDirectory: "keys/work", memberCount: 1, directoryPresent: true },
  ],
  metadata: {
    schemaVersion: 1,
    hosts: [
      { identity: { path: "connections/work.conf", alias: "database" }, tags: ["production"] },
    ],
  },
  diagnostics: [{ severity: "warning", code: "duplicate_alias" }],
  notices: [],
  pending: [],
} as Overview;

const sync = {
  configured: true,
  locked: false,
  endpoint: "https://example.invalid",
  bucket: "sshc",
  synced: true,
  direction: "both",
  lastSyncedAt: "2026-08-08T00:00:00Z",
  fileCount: 3,
} as SyncStatus;

describe("OverviewPanel", () => {
  it("puts the connection browser first and preserves attention and sync actions", async () => {
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue(overview)}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={vi.fn()}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    const browser = await screen.findByRole("region", { name: "Quick connect" });
    await within(browser).findByRole("searchbox", { name: "Search connections" });
    expect(screen.getByRole("heading", { name: "Home" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Quick connect" })).not.toBeInTheDocument();
    const attention = screen.getByRole("region", { name: "Needs attention" });
    expect(browser.compareDocumentPosition(attention) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    expect(within(attention).getByRole("button", { name: "Open SSH Config" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Open sync" })).toBeEnabled();
  });

  it("puts recent connections first, searches groups and launches only from the explicit action", async () => {
    const launch = vi.fn().mockResolvedValue({ session: { id: "session-1" } });
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue(overview)}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({
          connections: [{
            alias: "database",
            hostName: "old-db.example.com",
            user: "previous-user",
            port: "22",
            lastConnectedAt: "2026-08-24T15:30:00Z",
          }],
        })}
        launch={launch}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    const database = await screen.findByText("database");
    const cards = within(screen.getByRole("list", { name: "Available connections" })).getAllByRole("listitem");
    expect(database).toBeInTheDocument();
    expect(screen.getByText("bastion")).toBeInTheDocument();
    expect(screen.queryByText("Host *.internal")).toBeNull();
    expect(launch).not.toHaveBeenCalled();
    expect(cards[0]).toHaveTextContent("database");

    await userEvent.type(screen.getByRole("searchbox", { name: "Search connections" }), "production");
    expect(screen.getByText("database")).toBeInTheDocument();
    expect(screen.queryByText("bastion")).toBeNull();

    const card = screen.getByText("database").closest("li");
    expect(card).not.toBeNull();
    await userEvent.click(within(card as HTMLElement).getByRole("button", { name: "Actions for database" }));
    expect(launch).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("menuitem", { name: "Connect" }));
    await waitFor(() => expect(launch).toHaveBeenCalledWith("database"));
  });

  it("opens once from the focused card and reports readiness after the pending state", async () => {
    let finish!: (value: { session: { id: string } }) => void;
    const launch = vi.fn(() => new Promise<{ session: { id: string } }>((resolve) => { finish = resolve; }));
    const opened = vi.fn();
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue(overview)}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={launch}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
        onSessionOpened={opened}
      />,
    );

    const card = await screen.findByRole("button", { name: /^Connect to database\./ });
    card.focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Opening database…" })).toBeDisabled();
    const another = screen.getByRole("button", { name: /^Connect to bastion\./ });
    await userEvent.click(another);
    expect(launch).toHaveBeenCalledExactlyOnceWith("database");
    expect(opened).not.toHaveBeenCalled();

    await act(async () => finish({ session: { id: "session-1" } }));
    expect(opened).toHaveBeenCalledExactlyOnceWith("session-1");
    expect(another).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Opening database…" })).not.toBeInTheDocument();
  });

  it("opens the exact connection settings URL without launching", async () => {
    const launch = vi.fn();
    const navigateLocation = vi.fn();
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue(overview)}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={launch}
        onNavigate={vi.fn()}
        onNavigateLocation={navigateLocation}
      />,
    );

    const database = await screen.findByText("database");
    const card = database.closest("li");
    expect(card).not.toBeNull();
    await userEvent.click(within(card as HTMLElement).getByRole("button", { name: "Actions for database" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Open connection settings" }));

    expect(navigateLocation).toHaveBeenCalledWith(
      "/connections/servers?path=connections%2Fwork.conf&host=database&panel=basic",
    );
    expect(launch).not.toHaveBeenCalled();
  });

  it("routes configuration warnings to Config instead of an empty diagnostics form", async () => {
    const navigate = vi.fn();
    const loadOverview = vi.fn().mockResolvedValue(overview);
    render(
      <OverviewPanel
        loadOverview={loadOverview}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={vi.fn()}
        onNavigate={navigate}
        onNavigateLocation={vi.fn()}
      />,
    );

    await screen.findByText("database");
    await userEvent.click(screen.getByRole("button", { name: "Open SSH Config" }));
    expect(navigate).toHaveBeenCalledWith("Config");
    expect(screen.queryByRole("button", { name: "Open diagnostics" })).not.toBeInTheDocument();
    expect(loadOverview).toHaveBeenCalledTimes(1);
  });

  it("does not ask for attention for a Host * that only explains how connections get their values", async () => {
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue({
          ...overview,
          diagnostics: [],
          notices: [
            { code: "unnamed_host_block", path: "config", line: 26, detail: "Host *" },
            { code: "wildcard_shadow", path: "config", line: 26, detail: "Host *" },
          ],
        })}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={vi.fn()}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    await screen.findByText("database");
    expect(screen.queryByRole("region", { name: "Needs attention" })).not.toBeInTheDocument();
  });

  it("says what needs fixing and sends a group folder notice to Connections, with the button under the words", async () => {
    const navigate = vi.fn();
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue({
          ...overview,
          diagnostics: [],
          notices: [{ code: "group_directory_leftover", path: "connections/old" }],
        })}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={vi.fn()}
        onNavigate={navigate}
        onNavigateLocation={vi.fn()}
      />,
    );

    const attention = await screen.findByRole("region", { name: "Needs attention" });
    expect(attention).toHaveTextContent("1 notice(s) about connections or group folders need fixing.");
    expect(within(attention).queryByRole("button", { name: "Open SSH Config" })).not.toBeInTheDocument();
    const words = within(attention).getByText("1 notice(s) about connections or group folders need fixing.");
    const button = within(attention).getByRole("button", { name: "Open Connections" });
    expect(words.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    await userEvent.click(button);
    expect(navigate).toHaveBeenCalledWith("Connections");
  });

  it("reads the SSH configuration again when asked after it could not be read", async () => {
    const loadOverview = vi.fn()
      .mockRejectedValueOnce(new Error("config_failed"))
      .mockResolvedValue({ ...overview, diagnostics: [] });
    render(
      <OverviewPanel
        loadOverview={loadOverview}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("The SSH configuration could not be read.");
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));

    await screen.findByText("database");
    expect(loadOverview).toHaveBeenCalledTimes(2);
  });

  it("says connection history could not be read instead of showing none", async () => {
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue({ ...overview, diagnostics: [] })}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockRejectedValue(new Error("history_failed"))}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    expect(await screen.findByText("Connection history could not be read.")).toBeInTheDocument();
    expect(screen.getByText("database")).toBeInTheDocument();
  });

  it("keeps sync accessible without showing an empty attention section", async () => {
    const navigate = vi.fn();
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue({ ...overview, diagnostics: [] })}
        loadSync={vi.fn().mockResolvedValue({ ...sync, configured: false })}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        onNavigate={navigate}
        onNavigateLocation={vi.fn()}
      />,
    );
    await screen.findByText("database");
    expect(screen.queryByRole("region", { name: "Needs attention" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Open sync" }));
    expect(navigate).toHaveBeenCalledWith("Sync");
  });

  it("routes an interrupted write to History without showing a configuration action", async () => {
    const navigate = vi.fn();
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue({
          ...overview,
          diagnostics: [],
          pending: [{
            id: "tx-pending",
            operation: "config.save",
            status: "interrupted",
            startedAt: "2026-08-10T00:00:00Z",
            committed: 0,
            paths: ["config"],
            canComplete: false,
          }],
        })}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({ connections: [] })}
        launch={vi.fn()}
        onNavigate={navigate}
        onNavigateLocation={vi.fn()}
      />,
    );

    await screen.findByText("database");
    expect(screen.queryByRole("button", { name: "Open SSH Config" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Recover changes" }));
    expect(navigate).toHaveBeenCalledWith("History");
  });

  it("shows current targets for recent connections and reconnects from the action menu", async () => {
    const launch = vi.fn().mockResolvedValue({ session: { id: "session-1" } });
    render(
      <OverviewPanel
        loadOverview={vi.fn().mockResolvedValue(overview)}
        loadSync={vi.fn().mockResolvedValue(sync)}
        loadRecent={vi.fn().mockResolvedValue({
          connections: [{
            alias: "database",
            hostName: "old-db.example.com",
            user: "previous-user",
            port: "22",
            lastConnectedAt: "2026-08-24T15:30:00Z",
          }],
        })}
        launch={launch}
        onNavigate={vi.fn()}
        onNavigateLocation={vi.fn()}
      />,
    );

    const launcher = await screen.findByRole("list", { name: "Available connections" });
    const card = within(launcher).getByText("database").closest("li");
    expect(card).not.toBeNull();
    expect(within(card as HTMLElement).getByText("deploy@db.example.com:2202")).toBeInTheDocument();
    expect(card).toHaveTextContent("work");
    // The card shows the day as a badge; the whole time is its name and its tooltip.
    const lastConnected = within(card as HTMLElement).getByLabelText(`Last connected ${formatDateTime("2026-08-24T15:30:00Z", "en")}`);
    expect(lastConnected).toHaveAttribute("title", `Last connected ${formatDateTime("2026-08-24T15:30:00Z", "en")}`);
    expect(lastConnected.textContent).toMatch(/ago$/u);
    await userEvent.click(within(card as HTMLElement).getByRole("button", { name: "Actions for database" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Connect" }));
    await waitFor(() => expect(launch).toHaveBeenCalledWith("database"));
  });

});

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { HistoryPanel } from "./HistoryPanel";
import { configApi } from "../api/config";
import { ApiError } from "../api/client";

vi.mock("../api/config", async () => {
  const actual = await vi.importActual<typeof import("../api/config")>("../api/config");
  return { ...actual, configApi: { overview: vi.fn(), history: vi.fn(), restore: vi.fn(), recover: vi.fn(), pendingChanges: vi.fn() } };
});

// Monaco does not run in jsdom. The stand-in shows both sides as text so a test can read what is compared.
vi.mock("../editor/loadMonacoEditor", () => ({
  loadMonacoDiffEditor: () => Promise.resolve({
    MonacoDiffEditor: ({ original, modified }: { original: string; modified: string }) => (
      <div><pre data-testid="before">{original}</pre><pre data-testid="after">{modified}</pre></div>
    ),
  }),
}));

beforeEach(() => {
  vi.mocked(configApi.history).mockResolvedValue([{
    id: "20260805T120000.000-abcd",
    operation: "config.host_fields",
    status: "completed",
    startedAt: "2026-08-05T12:00:00Z",
    finishedAt: "2026-08-05T12:00:01Z",
    paths: ["config"],
    restorable: ["config"],
  }] as never);
  vi.mocked(configApi.overview).mockResolvedValue({
    entry: { path: "config", absolute: "/home/tester/.ssh/config" },
    files: [], hosts: [], metadata: { schemaVersion: 10, presets: [{ id: "default" }] }, diagnostics: [], notices: [],
    pending: [{
      id: "20260805T115900.000-ffff",
      operation: "config.file_raw",
      status: "staged",
      startedAt: "2026-08-05T11:59:00Z",
      committed: 1,
      paths: ["config", "conf.d/10-home.conf"],
      canComplete: true,
    }],
  } as never);
});

describe("HistoryPanel", () => {
  it("lists completed transactions and restores one file", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.restore).mockResolvedValue({
      transactionId: "t2", written: ["config"], preview: { operation: "config.restore", diffs: [] },
    });

    render(<HistoryPanel />);

    expect((await screen.findAllByText("Configuration change")).length).toBeGreaterThan(0);
    expect(screen.getByText("Completed")).toBeInTheDocument();
    expect(screen.queryByText("config.host_fields")).not.toBeInTheDocument();
    expect(screen.queryByText("completed")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Restore config" }));

    await waitFor(() => expect(configApi.restore).toHaveBeenCalledWith("20260805T120000.000-abcd", "config"));
  });

  it("shows an interrupted transaction as unfinished and offers both recoveries", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.recover).mockResolvedValue(undefined);

    render(<HistoryPanel />);

    expect(await screen.findByText(/1 of 2 files were written/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Roll back" }));

    await waitFor(() => expect(configApi.recover).toHaveBeenCalledWith("20260805T115900.000-ffff", "rollback"));
  });

  it("opens what the interrupted change writes, keeps a secret file's contents out, and rolls back from there", async () => {
    const user = userEvent.setup();
    vi.mocked(configApi.recover).mockResolvedValue(undefined);
    vi.mocked(configApi.pendingChanges).mockResolvedValue({
      id: "20260805T115900.000-ffff",
      files: [
        { path: "config", action: "write", committed: true, contents: "text", beforeExists: true, afterExists: true, before: "Host a\n  Port 22\n", after: "Host a\n  Port 2233\n" },
        { path: "sshc/local-vault-key", action: "write", committed: false, contents: "secret", beforeExists: true, afterExists: true },
      ],
    });

    render(<HistoryPanel />);
    await user.click(await screen.findByRole("button", { name: "View changes" }));

    const dialog = await screen.findByRole("dialog", { name: "What the interrupted change writes: Configuration change" });
    expect(configApi.pendingChanges).toHaveBeenCalledWith("20260805T115900.000-ffff");
    expect(await within(dialog).findByTestId("before")).toHaveTextContent("Port 22");
    expect(within(dialog).getByTestId("after")).toHaveTextContent("Port 2233");

    await user.click(within(dialog).getByRole("button", { name: /sshc\/local-vault-key/ }));
    expect(within(dialog).getByText("This file holds secrets, so its contents are not shown. Only that it changes is shown.")).toBeInTheDocument();
    expect(within(dialog).queryByTestId("before")).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Roll back" }));
    await waitFor(() => expect(configApi.recover).toHaveBeenCalledWith("20260805T115900.000-ffff", "rollback"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows when the interrupted change started as a date rather than the stored timestamp", async () => {
    render(<HistoryPanel />);

    expect(await screen.findByText(/1 of 2 files were written/)).not.toHaveTextContent("2026-08-05T11:59:00Z");
  });

  it("leaves the loading state and reports a history request failure", async () => {
    vi.mocked(configApi.history).mockRejectedValueOnce(new ApiError("history_unavailable", 500, null));

    render(<HistoryPanel />);

    expect(await screen.findByText("The request was rejected (history_unavailable)."))
      .toBeInTheDocument();
    expect(screen.queryByText("Loading history…")).not.toBeInTheDocument();
  });

  it("keeps completed history visible when the overview request fails", async () => {
    vi.mocked(configApi.overview).mockRejectedValueOnce(new ApiError("overview_unavailable", 500, null));

    render(<HistoryPanel />);

    expect((await screen.findAllByText("Configuration change")).length).toBeGreaterThan(0);
    expect(screen.getByText("The request was rejected (overview_unavailable)."))
      .toBeInTheDocument();
    expect(screen.queryByText("Loading history…")).not.toBeInTheDocument();
  });
});

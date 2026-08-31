import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { SyncPanel } from "./SyncPanel";
import type {
  IntegrationsApi,
  PullResponse,
  SyncStatus,
} from "../api/integrations";

afterEach(() => {
  vi.restoreAllMocks();
});

const unconfigured: SyncStatus = {
  configured: false,
  keyConfigured: false,
  locked: false,
  auto: { enabled: false, phase: "idle" },
  endpoint: "",
  bucket: "",
  synced: false,
  direction: "both",
};
const configured: SyncStatus = {
  configured: true,
  keyConfigured: true,
  locked: false,
  auto: { enabled: false, phase: "idle" },
  endpoint: "https://acc.r2.cloudflarestorage.com",
  bucket: "sshc",
  synced: true,
  direction: "both",
  lastSyncedAt: "2026-08-05T00:00:00Z",
  fileCount: 7,
};
const bucketStatus = {
  checkedAt: "2026-08-25T01:55:00Z",
  localIsLive: true,
  historyTruncated: false,
  live: {
    key: "workspace.tar.gz.enc",
    size: 900,
    lastModified: "2026-08-25T01:54:00Z",
  },
  history: [
    {
      key: "snapshots/2026-08-25-015400-aabbcc-000001.tar.gz.enc",
      size: 900,
      lastModified: "2026-08-25T01:54:00Z",
    },
  ],
};

const historyRevision = "a".repeat(64);
const historyStatus = {
  checkedAt: "2026-08-25T01:55:00Z",
  headRevision: historyRevision,
  historyTruncated: false,
  downloadTruncated: false,
  downloadedBytes: 1800,
  skipped: 0,
  revisions: [
    {
      key: "snapshots/2026-08-25-015400-aabbcc-000001.tar.gz.enc",
      revision: historyRevision,
      message: "Update config",
      createdAt: "2026-08-25T01:54:00Z",
      origin: "opaque-device-a",
      fileCount: 7,
      size: 900,
      lastModified: "2026-08-25T01:54:00Z",
      relation: "head" as const,
    },
  ],
};

function buildApi(
  status: SyncStatus,
  pull: PullResponse,
  overrides: Partial<IntegrationsApi> = {},
): IntegrationsApi {
  return {
    syncStatus: vi.fn().mockResolvedValue(status),
    checkSyncSetup: vi.fn().mockResolvedValue({
      state: "empty",
      historyPresent: false,
      checkedAt: "2026-08-26T00:00:00Z",
    }),
    completeSyncSetup: vi.fn().mockResolvedValue({
      status: { ...status, configured: true, keyConfigured: true },
      generatedKey: "ABCD-EFGH-JKMP-QRST-VWXY-Z012",
    }),
    syncPushDraft: vi.fn().mockResolvedValue({
      message: "Update config",
      added: 0,
      modified: 1,
      removed: 0,
    }),
    configureSync: vi.fn().mockResolvedValue({ ...status, configured: true }),
    syncExclusions: vi.fn().mockResolvedValue({
      document: "*.tmp\n",
      usingDefaults: true,
      candidates: [
        { path: "config", ignored: false },
        { path: "cache/session.tmp", ignored: true },
      ],
    }),
    saveSyncExclusions: vi.fn().mockResolvedValue({
      document: "*.tmp\n",
      usingDefaults: false,
      candidates: [],
    }),
    pushSnapshot: vi.fn().mockResolvedValue({
      status: { ...status, synced: true },
      result: {
        summary: measuredSummary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-12T01:02:04Z",
      },
    }),
    pullSnapshot: vi.fn().mockResolvedValue(pull),
    syncBucketStatus: vi.fn().mockResolvedValue(bucketStatus),
    syncHistory: vi.fn().mockResolvedValue(historyStatus),
    diffSyncHistory: vi.fn().mockResolvedValue({
      fromRevision: historyRevision,
      toRevision: historyRevision,
      added: [],
      modified: [],
      removed: [],
      downloadedBytes: 1800,
    }),
    forcePushSnapshot: vi.fn().mockResolvedValue({
      status,
      result: {
        summary: measuredSummary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-25T01:56:00Z",
      },
    }),
    ...overrides,
  } as unknown as IntegrationsApi;
}

const measuredSummary = {
  createdAt: "2026-08-12T01:02:03Z",
  fileCount: 7,
  sourceBytes: 1200,
  snapshotBytes: 900,
};

const nothingToDo: PullResponse = {
  applied: false,
  conflicts: [],
  written: [],
  removed: [],
  summary: measuredSummary,
  downloadedBytes: 900,
  completedAt: "2026-08-12T01:02:04Z",
  remoteETag: '"generation-1"',
  remoteRevision: historyRevision,
};

describe("SyncPanel", () => {
  it("stacks sync row labels above their controls on narrow screens", async () => {
    const { container } = render(
      <SyncPanel api={buildApi(unconfigured, nothingToDo)} />,
    );

    const endpoint = await screen.findByLabelText("Endpoint");
    expect(endpoint.closest("label")).toHaveClass("flex-col", "sm:flex-row");
    expect(container.firstElementChild).toHaveClass(
      "[&_button]:min-h-10",
      "md:[&_button]:min-h-0",
    );
  });

  it("shows a retry action when the sync status cannot be loaded", async () => {
    const syncStatus = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(configured);
    const api = buildApi(configured, nothingToDo, { syncStatus });

    render(<SyncPanel api={api} />);

    expect(screen.getByRole("status")).toHaveTextContent(
      "Reading the sync settings",
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The sync settings could not be read",
    );
    expect(
      screen.queryByText("Reading the sync settings…"),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(
      await screen.findByText("https://acc.r2.cloudflarestorage.com/sshc"),
    ).toBeInTheDocument();
    expect(syncStatus).toHaveBeenCalledTimes(2);
  });

  it("says what travels before the form asks for anything", async () => {
    render(<SyncPanel api={buildApi(unconfigured, nothingToDo)} />);

    expect(
      await screen.findByText(/including private keys/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/guess the encryption key offline/),
    ).toBeInTheDocument();
  });

  it("reads the current object and dated history from the bucket", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByRole("heading", { name: "Bucket status" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("workspace.tar.gz.enc")).toBeInTheDocument();
    expect(
      screen.getByText("snapshots/2026-08-25-015400-aabbcc-000001.tar.gz.enc"),
    ).toBeInTheDocument();
    expect(api.syncBucketStatus).toHaveBeenCalledTimes(1);

    await userEvent.click(
      screen.getByRole("button", { name: "Refresh bucket status" }),
    );
    await waitFor(() => expect(api.syncBucketStatus).toHaveBeenCalledTimes(2));
  });

  it("shows only the five newest bucket objects until history is expanded", async () => {
    const history = Array.from({ length: 8 }, (_, index) => ({
      key: `snapshots/history-${index}.tar.gz.enc`,
      size: 900 + index,
      lastModified: `2026-08-25T01:${String(59 - index).padStart(2, "0")}:00Z`,
    }));
    const api = buildApi(configured, nothingToDo, {
      syncBucketStatus: vi.fn().mockResolvedValue({
        ...bucketStatus,
        history,
      }),
    });
    render(<SyncPanel api={api} />);

    expect(await screen.findByText(/latest 5 of 8 entries/i)).toBeInTheDocument();
    expect(screen.getByText("snapshots/history-4.tar.gz.enc")).toBeInTheDocument();
    expect(screen.queryByText("snapshots/history-5.tar.gz.enc")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Show all history" }));
    expect(screen.getByText("snapshots/history-7.tar.gz.enc")).toBeInTheDocument();
  });

  it("does not offer a push when the local workspace is unchanged", async () => {
    const api = buildApi(configured, nothingToDo, {
      syncPushDraft: vi.fn().mockResolvedValue({
        message: "Record current workspace",
        added: 0,
        modified: 0,
        removed: 0,
      }),
    });
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByText("There are no local changes to push."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Push this workspace" }),
    ).toBeDisabled();
  });

  it("compares and restores one encrypted revision without rewinding the remote head", async () => {
    const historyKey = "snapshots/2026-08-25-015400-aabbcc-000001.tar.gz.enc";
    const pullSnapshot = vi.fn().mockResolvedValue({
      ...nothingToDo,
      written: ["config"],
    });
    const diffSyncHistory = vi.fn().mockResolvedValue({
      fromRevision: historyRevision,
      toRevision: "b".repeat(64),
      added: ["connections/new.conf"],
      modified: ["config"],
      removed: ["connections/old.conf"],
      downloadedBytes: 1800,
    });
    const api = buildApi(configured, nothingToDo, {
      pullSnapshot,
      diffSyncHistory,
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /aaaaaaaaaaaa.*HEAD/i }),
    );
    await waitFor(() =>
      expect(diffSyncHistory).toHaveBeenCalledWith(historyKey),
    );
    expect(await screen.findByText("Added · 1")).toBeInTheDocument();
    expect(screen.getByText("Modified · 1")).toBeInTheDocument();
    expect(screen.getByText("Removed · 1")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Preview restoring this revision" }),
    );
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        false,
        undefined,
        historyKey,
      ),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Apply the snapshot" }),
    );
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        true,
        undefined,
        historyKey,
        expect.objectContaining({ remoteRevision: historyRevision }),
      ),
    );
  });

  it("requires explicit confirmation before replacing the remote snapshot", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Force send" }),
    );
    const replace = screen.getByRole("button", {
      name: "Replace the remote snapshot",
    });
    expect(replace).toBeDisabled();
    await userEvent.click(
      screen.getByRole("checkbox", { name: /current remote snapshot/i }),
    );
    expect(replace).toBeEnabled();
    await userEvent.click(replace);
    await waitFor(() =>
      expect(api.forcePushSnapshot).toHaveBeenCalledWith("Update config"),
    );
  });

  it("traps focus in the force-send dialog and restores it after Escape", async () => {
    const user = userEvent.setup();
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    const opener = await screen.findByRole("button", { name: "Force send" });
    await user.click(opener);
    const dialog = screen.getByRole("dialog");
    const close = within(dialog).getByRole("button", { name: "Close" });
    const confirmation = within(dialog).getByRole("checkbox", { name: /current remote snapshot/i });
    await waitFor(() => expect(close).toHaveFocus());

    await user.tab({ shift: true });
    expect(confirmation).toHaveFocus();
    await user.tab();
    expect(close).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(opener).toHaveFocus());
  });

  it("starts with the generated message and sends the user's edit", async () => {
    const pushSnapshot = vi.fn().mockResolvedValue({
      status: configured,
      result: {
        summary: measuredSummary,
        objectCount: 2,
        uploadedBytes: 1800,
        completedAt: "2026-08-25T02:00:00Z",
      },
    });
    const api = buildApi(configured, nothingToDo, { pushSnapshot });
    render(<SyncPanel api={api} />);

    const message = await screen.findByLabelText("Commit message");
    expect(message).toHaveValue("Update config");
    await userEvent.clear(message);
    expect(
      screen.getByRole("button", { name: "Push this workspace" }),
    ).toBeDisabled();
    await userEvent.type(message, "Refresh production hosts");
    expect(
      screen.getByRole("button", { name: "Push this workspace" }),
    ).toBeEnabled();
    await userEvent.click(
      screen.getByRole("button", { name: "Push this workspace" }),
    );
    await waitFor(() =>
      expect(pushSnapshot).toHaveBeenCalledWith("Refresh production hosts"),
    );
    expect(await screen.findByText("Update config")).toBeInTheDocument();
  });

  it("configures a bucket and clears the credentials from the form", async () => {
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://acc.r2.cloudflarestorage.com",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Verify and save" }),
    );

    await waitFor(() =>
      expect(api.completeSyncSetup).toHaveBeenCalledWith(
        expect.objectContaining({
          endpoint: "https://acc.r2.cloudflarestorage.com",
          bucket: "sshc",
          path: "",
          region: "",
          accessKeyId: "AKID",
          secretAccessKey: "the-secret",
          direction: "both",
          expectedState: "empty",
        }),
      ),
    );
    await waitFor(() =>
      expect(
        screen.queryByLabelText("Secret access key"),
      ).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "Edit bucket settings" }),
    ).toBeInTheDocument();
    expect(document.body.textContent ?? "").not.toContain("the-secret");
  });

  it("keeps configured credentials collapsed until they need editing", async () => {
    render(
      <SyncPanel api={buildApi(configured, nothingToDo)} />,
    );

    await screen.findByText("Manage sync settings");
    const summary = screen
      .getByText("Manage sync settings")
      .closest("summary") as HTMLElement;
    expect(summary).not.toBeNull();
    const settings = summary.closest("details");
    expect(settings).not.toHaveAttribute("open");
    expect(summary.querySelector("[aria-hidden='true']")).toHaveTextContent(
      "›",
    );
    expect(
      screen.queryByLabelText("Secret access key"),
    ).not.toBeInTheDocument();
    await userEvent.click(summary);
    expect(settings).toHaveAttribute("open");
    expect(settings).toContainElement(
      screen.getByRole("heading", { name: "Bucket" }),
    );
    expect(settings).toContainElement(
      screen.getByRole("heading", { name: "Encryption key" }),
    );
    expect(settings).not.toContainElement(
      screen.getByText("Details and history").closest("summary"),
    );
    expect(
      screen.getByText("https://acc.r2.cloudflarestorage.com/sshc"),
    ).toBeInTheDocument();
    await userEvent.click(
      await screen.findByRole("button", { name: "Edit bucket settings" }),
    );
    expect(screen.getByLabelText("Endpoint")).toHaveValue(
      "https://acc.r2.cloudflarestorage.com",
    );
    expect(screen.getByLabelText("Bucket name")).toHaveValue("sshc");
    expect(screen.getByLabelText("Secret access key")).toHaveValue("");
  });

  it("sends the region it was given", async () => {
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://s3.eu-west-2.amazonaws.com",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Region"), "eu-west-2");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );

    await waitFor(() =>
      expect(api.checkSyncSetup).toHaveBeenCalledWith(
        expect.objectContaining({ region: "eu-west-2" }),
      ),
    );
  });

  it("does not show transfer controls before setup is complete", async () => {
    render(<SyncPanel api={buildApi(unconfigured, nothingToDo)} />);

    expect(
      await screen.findByRole("button", { name: "Check connection" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Push this workspace" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Check for changes" }),
    ).not.toBeInTheDocument();
  });

  it("previews before it applies", async () => {
    const api = buildApi(configured, {
      ...nothingToDo,
      applied: false,
      conflicts: [],
      written: ["config", "connections/work/lon.conf"],
      removed: ["connections/old.conf"],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );

    await waitFor(() =>
      expect(api.pullSnapshot).toHaveBeenCalledWith(
        false,
        undefined,
        undefined,
      ),
    );
    expect(
      await screen.findByText("connections/work/lon.conf"),
    ).toBeInTheDocument();
    expect(screen.getByText("connections/old.conf")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("checkbox", { name: /overwrite files in ~\/.ssh/i }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Apply the snapshot" }),
    );
    await waitFor(() =>
      expect(api.pullSnapshot).toHaveBeenLastCalledWith(
        true,
        undefined,
        undefined,
        expect.objectContaining({ remoteRevision: historyRevision }),
      ),
    );
  });

  it("traps focus in the pull preview and restores it after Escape", async () => {
    const user = userEvent.setup();
    const api = buildApi(configured, {
      ...nothingToDo,
      applied: false,
      conflicts: [],
      written: ["config"],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    const opener = await screen.findByRole("button", { name: "Check for changes" });
    await user.click(opener);
    const dialog = await screen.findByRole("dialog");
    const close = within(dialog).getByRole("button", { name: "Close" });
    const apply = within(dialog).getByRole("button", { name: "Apply the snapshot" });
    await waitFor(() => expect(close).toHaveFocus());

    await user.tab({ shift: true });
    expect(apply).toHaveFocus();
    await user.tab();
    expect(close).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(opener).toHaveFocus());
  });

  it("shows a conflict and refuses to apply it", async () => {
    const api = buildApi(configured, {
      ...nothingToDo,
      applied: false,
      conflicts: [
        {
          path: "config",
          changedHere: true,
          changedThere: true,
          baseMode: "0600",
          localMode: "0700",
          remoteMode: "0600",
        },
      ],
      written: [],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );

    expect(
      await screen.findByText(/changed on this machine and another machine/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Permissions: last sync 0600 · this machine 0700 · remote 0600",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Apply the snapshot" }),
    ).toBeDisabled();
  });

  it("says so when there is nothing to do", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );

    expect(
      await screen.findByText(/already matches the snapshot/),
    ).toBeInTheDocument();
  });

  it("shows the measured push result instead of only saying it succeeded", async () => {
    const api = buildApi(configured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Push this workspace" }),
    );

    expect(
      await screen.findByRole("heading", { name: "This push" }),
    ).toBeInTheDocument();
    expect(screen.getByText("7 files · 1.2 kB")).toBeInTheDocument();
    expect(
      screen.getByText("S3 transfer 1.8 kB (2 objects, history + live)"),
    ).toBeInTheDocument();
  });

  it("shows preview and apply as two separately measured downloads", async () => {
    const preview = { ...nothingToDo, written: ["config"] };
    const applied = {
      ...preview,
      applied: true,
      completedAt: "2026-08-12T01:03:00Z",
    };
    const api = buildApi(configured, preview, {
      pullSnapshot: vi
        .fn()
        .mockResolvedValueOnce(preview)
        .mockResolvedValueOnce(applied),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );
    expect(
      await screen.findByRole("heading", { name: "Pull preview" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Downloaded 900 B · 1.2 kB after opening"),
    ).toBeInTheDocument();

    await userEvent.click(
      await screen.findByRole("button", { name: "Apply the snapshot" }),
    );
    expect(
      await screen.findByRole("heading", { name: "Apply result" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Downloaded again for apply: 900 B"),
    ).toBeInTheDocument();
  });

  it("shows a persisted operation as the previous success after reload", async () => {
    const lastOperation = {
      kind: "push" as const,
      summary: measuredSummary,
      objectCount: 2,
      uploadedBytes: 1800,
      completedAt: "2026-08-12T01:02:04Z",
    };
    render(
      <SyncPanel
        api={buildApi({ ...configured, lastOperation }, nothingToDo)}
      />,
    );

    expect(
      await screen.findByRole("heading", { name: "Previous success" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("S3 transfer 1.8 kB (2 objects, history + live)"),
    ).toBeInTheDocument();
  });

  it("keeps the previous success separate when a later push fails partway", async () => {
    const lastOperation = {
      kind: "push" as const,
      summary: measuredSummary,
      objectCount: 2,
      uploadedBytes: 1800,
      completedAt: "2026-08-12T01:02:04Z",
    };
    const api = buildApi({ ...configured, lastOperation }, nothingToDo, {
      pushSnapshot: vi
        .fn()
        .mockRejectedValue(new ApiError("sync_remote_moved", 409, null)),
    });
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByRole("heading", { name: "Previous success" }),
    ).toBeInTheDocument();
    await userEvent.click(
      await screen.findByRole("button", { name: "Push this workspace" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /current snapshot.*update was cancelled/i,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      /cancelled before retrying/i,
    );
    expect(
      screen.getByRole("heading", { name: "Previous success" }),
    ).toBeInTheDocument();
  });

  it("sends the chosen direction with the bucket", async () => {
    const api = buildApi(unconfigured, nothingToDo);
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://acc.r2.cloudflarestorage.com",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.selectOptions(screen.getByLabelText("Direction"), "pull");
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Verify and save" }),
    );

    await waitFor(() =>
      expect(api.completeSyncSetup).toHaveBeenCalledWith(
        expect.objectContaining({ direction: "pull" }),
      ),
    );
  });

  it("offers no push on a machine set to receive only, and says why", async () => {
    const syncNow = vi
      .fn()
      .mockResolvedValue({ ...configured, direction: "pull" });
    const api = buildApi({ ...configured, direction: "pull" }, nothingToDo, {
      syncNow,
    });
    render(<SyncPanel api={api} />);

    const sync = await screen.findByRole("button", { name: "Receive now" });
    expect(
      screen.getByRole("button", { name: "Review remote changes" }),
    ).toBeEnabled();
    expect(
      screen.queryByRole("button", { name: "Push this workspace" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Commit message")).not.toBeInTheDocument();
    expect(
      screen.getByText(/Changes on this machine are never pushed/),
    ).toBeInTheDocument();
    expect(api.syncPushDraft).not.toHaveBeenCalled();

    await userEvent.click(sync);
    await waitFor(() => expect(syncNow).toHaveBeenCalledOnce());
  });

  it("lets a receive-only machine explicitly review and accept a diverged remote head", async () => {
    const status: SyncStatus = {
      ...configured,
      direction: "pull",
      auto: { enabled: false, phase: "blocked", detail: "remote_moved" },
    };
    const remote = {
      ...nothingToDo,
      written: ["config"],
      origin: "remote-device-2",
    };
    const pullSnapshot = vi.fn().mockResolvedValue(remote);
    const api = buildApi(status, remote, { pullSnapshot });
    render(<SyncPanel api={api} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /diverged.*last revision received/i,
    );
    expect(
      screen.queryByRole("button", { name: "Sync now" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Receive from remote" }),
    ).not.toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Review current remote" }),
    );
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        false,
        "remote",
        undefined,
        undefined,
        true,
      ),
    );
    expect(
      await screen.findByRole("heading", { name: "Receive current remote" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/remote-device-2/)).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Receive this remote" }),
    );
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        true,
        "remote",
        undefined,
        expect.objectContaining({ remoteRevision: historyRevision }),
        true,
      ),
    );
  });

  it("previews the current remote before an explicit force receive", async () => {
    const remote = {
      ...nothingToDo,
      written: ["config"],
      origin: "remote-device-2",
    };
    const pullSnapshot = vi.fn().mockResolvedValue(remote);
    const api = buildApi(configured, remote, { pullSnapshot });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Force receive" }),
    );

    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        false,
        "remote",
        undefined,
        undefined,
        true,
      ),
    );
    expect(
      await screen.findByRole("heading", { name: "Receive current remote" }),
    ).toBeInTheDocument();
    expect(screen.getByText("config")).toBeInTheDocument();
  });

  it("keeps the specific receive-only recovery state instead of adding a generic error", async () => {
    const status: SyncStatus = {
      ...configured,
      direction: "pull",
      auto: { enabled: false, phase: "blocked", detail: "remote_moved" },
    };
    const syncStatus = vi.fn().mockResolvedValue(status);
    const api = buildApi(status, nothingToDo, {
      syncStatus,
      pullSnapshot: vi
        .fn()
        .mockRejectedValue(new ApiError("sync_remote_moved", 409, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Review current remote" }),
    );

    await waitFor(() => expect(syncStatus).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("alert")).toHaveTextContent(
      /diverged.*last revision received/i,
    );
    expect(
      screen.queryByText(/snapshot could not be received/i),
    ).not.toBeInTheDocument();
  });

  it("offers no apply on a machine set to send only, but still shows what would change", async () => {
    const api = buildApi(
      { ...configured, direction: "push" },
      {
        ...nothingToDo,
        applied: false,
        conflicts: [],
        written: ["config"],
        removed: [],
      },
    );
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );

    expect(await screen.findByText("config")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Apply the snapshot" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Push this workspace" }),
    ).toBeEnabled();
  });

  it("reports a refused push instead of claiming success", async () => {
    const api = buildApi(configured, nothingToDo, {
      pushSnapshot: vi.fn().mockRejectedValue(new Error("sync_remote_moved")),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Push this workspace" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /could not be uploaded|download those changes first/i,
    );
  });
  it("asks for the master password rather than showing an empty bucket form", async () => {
    const api = buildApi({ ...unconfigured, locked: true }, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(await screen.findByLabelText("Master password")).toBeInTheDocument();
    expect(screen.queryByLabelText("Access key ID")).not.toBeInTheDocument();
    expect(
      screen.getByText(/encrypted with the master password/i),
    ).toBeInTheDocument();
  });

  it("opens the vault in place and reads the settings back", async () => {
    const syncStatus = vi
      .fn()
      .mockResolvedValueOnce({ ...unconfigured, locked: true })
      .mockResolvedValue(configured);
    const api = buildApi(unconfigured, nothingToDo, {
      syncStatus,
      unlockVault: vi.fn().mockResolvedValue({
        exists: true,
        unlocked: true,
        aliases: [],
        dedicatedKeyPassphrases: [],
      }),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Master password"),
      "the master password",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Unlock" }),
    );

    await waitFor(() =>
      expect(api.unlockVault).toHaveBeenCalledWith("the master password"),
    );
    expect(
      await screen.findByText("https://acc.r2.cloudflarestorage.com/sshc"),
    ).toBeInTheDocument();
  });
  it("points at the key, not the master password, when the snapshot does not open", async () => {
    const api = buildApi(configured, nothingToDo, {
      pushSnapshot: vi
        .fn()
        .mockRejectedValue(new ApiError("wrong_passphrase", 403, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Push this workspace" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /saved key cannot decrypt/i,
    );
  });

  it("shows a generated key once and never asks for it again", async () => {
    const setSyncKey = vi
      .fn()
      .mockResolvedValue({ key: "AB12-CD34-EF56-GH78-JK90-MN12" });
    const syncStatus = vi
      .fn()
      .mockResolvedValueOnce({ ...configured, keyConfigured: false })
      .mockResolvedValue(configured);
    const api = buildApi(configured, nothingToDo, { setSyncKey, syncStatus });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Create a key" }),
    );

    await waitFor(() => expect(setSyncKey).toHaveBeenCalledWith(undefined));
    expect(
      await screen.findByText("AB12-CD34-EF56-GH78-JK90-MN12"),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Key")).not.toBeInTheDocument();
  });

  it("takes a key the person chose without echoing it back", async () => {
    const setSyncKey = vi
      .fn()
      .mockResolvedValue({ key: "a key chosen by hand" });
    const api = buildApi({ ...configured, keyConfigured: false }, nothingToDo, {
      setSyncKey,
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByLabelText("Choose the key myself"),
    );
    await userEvent.type(screen.getByLabelText("Key"), "a key chosen by hand");
    await userEvent.click(
      await screen.findByRole("button", { name: "Create a key" }),
    );

    await waitFor(() =>
      expect(setSyncKey).toHaveBeenCalledWith("a key chosen by hand"),
    );
    expect(screen.queryByText("a key chosen by hand")).not.toBeInTheDocument();
  });

  it("tells the user to re-enter the same new key after an interrupted rotation", async () => {
    const setSyncKey = vi
      .fn()
      .mockRejectedValue(new ApiError("sync_key_recovery_required", 409, null));
    const api = buildApi(configured, nothingToDo, {
      setSyncKey,
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByLabelText("Choose the key myself"),
    );
    await userEvent.type(
      screen.getByLabelText("Key"),
      "the same new synchronization key",
    );
    const replace = screen.getByRole("button", { name: "Replace the key" });
    expect(replace).toBeDisabled();
    await userEvent.click(
      screen.getByLabelText(/older history snapshots will remain encrypted/i),
    );
    await userEvent.click(replace);

    await waitFor(() =>
      expect(setSyncKey).toHaveBeenCalledWith(
        "the same new synchronization key",
        true,
      ),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /same new synchronization key again/i,
    );
  });

  it("offers no push or pull until a key exists", async () => {
    const api = buildApi({ ...configured, keyConfigured: false }, nothingToDo);
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByRole("button", { name: "Push this workspace" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Check for changes" }),
    ).toBeDisabled();
  });

  it("says nothing was saved when the bucket did not answer", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      checkSyncSetup: vi
        .fn()
        .mockRejectedValue(new ApiError("bucket_refused", 502, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://acc.r2.cloudflarestorage.com",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /Nothing was saved/i,
    );
  });

  it("explains an object store authentication failure specifically", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      checkSyncSetup: vi
        .fn()
        .mockRejectedValue(
          new ApiError("bucket_authentication_failed", 502, null),
        ),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://acc.r2.cloudflarestorage.com",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "bad-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/could not authenticate/i);
    expect(alert).toHaveTextContent("Code: bucket_authentication_failed");
  });

  it("shows a specific network cause and a stable diagnostic code", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      checkSyncSetup: vi
        .fn()
        .mockRejectedValue(new ApiError("bucket_dns_failed", 502, null)),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://misspelled.invalid",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/hostname could not be resolved/i);
    expect(alert).toHaveTextContent("Code: bucket_dns_failed");
  });

  it("keeps the automatic cycle failure cause visible after the request returns", async () => {
    const api = buildApi(
      {
        ...configured,
        auto: {
          enabled: true,
          phase: "failed",
          detail: "bucket_dns_failed",
        },
      },
      nothingToDo,
    );
    render(<SyncPanel api={api} />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/hostname could not be resolved/i);
    expect(alert).toHaveTextContent("Code: bucket_dns_failed");
  });

  it("explains an endpoint that carries a path instead of just refusing it", async () => {
    const api = buildApi(unconfigured, nothingToDo, {
      checkSyncSetup: vi
        .fn()
        .mockRejectedValue(
          new ApiError("endpoint_must_have_no_path", 400, null),
        ),
    });
    render(<SyncPanel api={api} />);

    await userEvent.type(
      await screen.findByLabelText("Endpoint"),
      "https://acc.r2.cloudflarestorage.com/sshc",
    );
    await userEvent.type(screen.getByLabelText("Bucket name"), "sshc");
    await userEvent.type(screen.getByLabelText("Access key ID"), "AKID");
    await userEvent.type(
      screen.getByLabelText("Secret access key"),
      "the-secret",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Check connection" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /no bucket name and no path/i,
    );
  });
  it("will not apply a pull that removes files until it is told to go ahead", async () => {
    const removing = {
      applied: false,
      summary: {
        createdAt: "2026-08-12T01:30:00Z",
        fileCount: 2,
        sourceBytes: 10,
        snapshotBytes: 20,
      },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      remoteETag: '"generation-1"',
      remoteRevision: historyRevision,
      conflicts: [],
      written: [],
      removed: ["~/.ssh/connections/old.conf"],
    };
    const api = buildApi(configured, removing);
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );
    const apply = await screen.findByRole("button", {
      name: "Apply the snapshot",
    });
    expect(apply).toBeDisabled();

    await userEvent.click(
      screen.getByRole("checkbox", { name: /overwrite files in ~\/.ssh/i }),
    );
    expect(apply).toBeEnabled();
  });

  it("applies a pull that only writes without asking again", async () => {
    const api = buildApi(configured, {
      applied: false,
      summary: {
        createdAt: "2026-08-12T01:30:00Z",
        fileCount: 2,
        sourceBytes: 10,
        snapshotBytes: 20,
      },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      remoteETag: '"generation-1"',
      remoteRevision: historyRevision,
      conflicts: [],
      written: ["~/.ssh/config"],
      removed: [],
    });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );
    expect(
      await screen.findByRole("button", { name: "Apply the snapshot" }),
    ).toBeEnabled();
    expect(
      screen.queryByRole("checkbox", { name: /overwrites files/i }),
    ).not.toBeInTheDocument();
  });

  it("says what the loop is waiting for instead of only that it stopped", async () => {
    const api = buildApi(
      {
        ...configured,
        auto: {
          enabled: true,
          phase: "blocked",
          detail: "removals",
          at: "2026-08-18T00:00:00Z",
        },
      },
      nothingToDo,
    );
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByText(/would remove files from this machine/i),
    ).toBeInTheDocument();
  });

  it("explains that send-only auto sync stopped before uploading a moved remote", async () => {
    const api = buildApi(
      {
        ...configured,
        auto: {
          enabled: true,
          phase: "blocked",
          detail: "remote_moved",
          at: "2026-08-25T09:00:00Z",
        },
      },
      nothingToDo,
    );
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByText(
        /stopped before uploading because another machine changed/i,
      ),
    ).toBeInTheDocument();
  });

  it("explains that an acknowledged live snapshot was deleted", async () => {
    const api = buildApi(
      {
        ...configured,
        auto: {
          enabled: true,
          phase: "blocked",
          detail: "remote_deleted",
          at: "2026-08-25T09:00:00Z",
        },
      },
      nothingToDo,
    );
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByText(/live snapshot was deleted from the bucket/i),
    ).toBeInTheDocument();
  });

  it("turns the loop on and keeps what the server answered", async () => {
    const setAutoSync = vi.fn().mockResolvedValue({
      ...configured,
      auto: { enabled: true, phase: "idle" },
    });
    const api = buildApi(configured, nothingToDo, { setAutoSync });
    render(<SyncPanel api={api} />);

    expect(
      await screen.findByText(/checks the remote once a minute/i),
    ).toHaveTextContent(/five seconds pass without another change/i);

    await userEvent.click(
      await screen.findByRole("checkbox", {
        name: /Keep this machine in sync/i,
      }),
    );

    await waitFor(() => expect(setAutoSync).toHaveBeenCalledWith(true));
    expect(
      await screen.findByRole("checkbox", {
        name: /Keep this machine in sync/i,
      }),
    ).toBeChecked();
  });

  it("offers a manual cycle while the loop is off", async () => {
    const syncNow = vi.fn().mockResolvedValue(configured);
    const api = buildApi(configured, nothingToDo, { syncNow });
    render(<SyncPanel api={api} />);

    const button = await screen.findByRole("button", { name: "Sync now" });
    expect(button).toBeEnabled();
    await userEvent.click(button);
    await waitFor(() => expect(syncNow).toHaveBeenCalledOnce());
  });

  it("offers both sides of a conflict and previews the choice before applying it", async () => {
    const conflicted = {
      applied: false,
      summary: {
        createdAt: "2026-08-12T01:30:00Z",
        fileCount: 1,
        sourceBytes: 10,
        snapshotBytes: 20,
      },
      downloadedBytes: 20,
      completedAt: "2026-08-12T01:31:00Z",
      remoteETag: '"generation-1"',
      remoteRevision: historyRevision,
      conflicts: [{ path: "config", changedHere: true, changedThere: true }],
      written: [],
      removed: [],
    };
    const resolved = { ...conflicted, conflicts: [], written: ["config"] };
    const pullSnapshot = vi
      .fn()
      .mockResolvedValueOnce(conflicted)
      .mockResolvedValue(resolved);
    const api = buildApi(configured, conflicted, { pullSnapshot });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Use the other machine's version",
      }),
    );

    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(false, "remote", undefined),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Apply the snapshot" }),
    );
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        true,
        "remote",
        undefined,
        resolved,
      ),
    );
  });

  it("can acknowledge a local conflict choice even when it writes no files", async () => {
    const conflicted = {
      ...nothingToDo,
      conflicts: [{ path: "config", changedHere: true, changedThere: true }],
    };
    const keepLocal = { ...conflicted, conflicts: [] };
    const pullSnapshot = vi
      .fn()
      .mockResolvedValueOnce(conflicted)
      .mockResolvedValue(keepLocal);
    const api = buildApi(configured, conflicted, { pullSnapshot });
    render(<SyncPanel api={api} />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Check for changes" }),
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Keep this machine's version",
      }),
    );

    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(false, "local", undefined),
    );
    const apply = await screen.findByRole("button", {
      name: "Apply the snapshot",
    });
    expect(apply).toBeEnabled();
    await userEvent.click(apply);
    await waitFor(() =>
      expect(pullSnapshot).toHaveBeenLastCalledWith(
        true,
        "local",
        undefined,
        keepLocal,
      ),
    );
  });
});

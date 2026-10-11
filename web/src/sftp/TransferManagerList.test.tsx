import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/messages";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { TransferManagerList } from "./TransferManagerList";

type Job = {
  id: string;
  status: string;
  direction: string;
  problem: string;
  allowedActions: string[];
  operation: string;
  sourceAlias: string;
  sourcePath: string;
  alias: string;
  remotePath: string;
};

const manager = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  let jobs: unknown[] = [];
  return {
    listeners,
    setJobs(next: unknown[]) {
      jobs = next;
      for (const listener of listeners) listener();
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    getSnapshot: () => jobs,
    getProcessingStopped: vi.fn(() => false),
    getMaxReconnectAttempts: vi.fn(() => 0),
    getExcludePatterns: vi.fn(() => [] as string[]),
    hasUploadSource: vi.fn(() => true),
    applySettings: vi.fn(async () => undefined),
    move: vi.fn(async () => undefined),
    pause: vi.fn(async () => undefined),
    resume: vi.fn(async () => undefined),
    retry: vi.fn(async () => undefined),
    cancel: vi.fn(async () => undefined),
    overwrite: vi.fn(async () => undefined),
    pauseAll: vi.fn(async () => undefined),
    resumeAll: vi.fn(async () => undefined),
    cancelAll: vi.fn(async () => undefined),
    clearFinished: vi.fn(async () => undefined),
    clearFailed: vi.fn(async () => undefined),
    remove: vi.fn(async () => undefined),
    reconcile: vi.fn(async () => undefined),
    retryFailed: vi.fn(async () => undefined),
  };
});

vi.mock("./transferManager", () => ({ sftpTransferManager: manager }));

function job(id: string, overrides: Partial<Job> = {}) {
  return {
    id,
    batchId: "batch_one",
    batchName: "batch",
    batchKind: "file",
    alias: "edge",
    direction: "download",
    kind: "file",
    name: id,
    remotePath: `/${id}`,
    totalBytes: 100,
    transferredBytes: 0,
    bytesPerSecond: 0,
    remainingSeconds: -1,
    status: "queued",
    allowedActions: ["pause", "cancel"],
    attempt: 1, reconnectAttempt: 0, reconnectAt: "",
    problem: "",
    operation: "", sourceAlias: "", sourcePath: "",
    lastModified: 0,
    expectedRevision: "",
    sourceFingerprint: "",
    overwrite: false,
    downloadRevision: "",
    downloadParts: [],
    createdAt: "",
    updatedAt: "",
    ...overrides,
  };
}

describe("the transfer queue", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.localStorage.setItem("sshc.sftp.queueView", JSON.stringify({ collapsed: false, height: 224 }));
    vi.clearAllMocks();
    manager.getProcessingStopped.mockReturnValue(false);
    manager.getMaxReconnectAttempts.mockReturnValue(0);
    manager.getExcludePatterns.mockReturnValue([]);
    manager.hasUploadSource.mockReturnValue(true);
    manager.setJobs([]);
  });

  it("shows one name for a single transfer and keeps its saved exclusions in the details", async () => {
    manager.getExcludePatterns.mockReturnValue(["node_modules"]);
    manager.setJobs([{ ...job("report.txt", { status: "completed", allowedActions: ["remove"] }), batchName: "report.txt", excludePatterns: [".git"] }]);
    render(<TransferManagerList />);
    expect(screen.getAllByText("report.txt")).toHaveLength(1);
    expect(screen.queryByText("1 exclusion rules")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show details for report.txt" }));
    expect(screen.getByText("1 exclusion rules")).toHaveAttribute("title", expect.stringContaining(".git"));
    expect(screen.queryByText("node_modules")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Hide details for report.txt" }));
    expect(screen.queryByText("1 exclusion rules")).not.toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeVisible();
  });

  it("goes to Settings > SFTP from a folded empty queue without unfolding it", async () => {
    window.localStorage.setItem("sshc.sftp.queueView", JSON.stringify({ collapsed: true, height: 224 }));
    const navigate = vi.fn();
    render(<TransferManagerList onNavigateLocation={navigate} />);
    await userEvent.click(screen.getByRole("button", { name: "Transfer settings" }));
    expect(navigate).toHaveBeenCalledWith("/settings/sftp");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Split at")).not.toBeInTheDocument();
    expect(JSON.parse(window.localStorage.getItem("sshc.sftp.queueView") ?? "{}")).toMatchObject({ collapsed: true, height: 224 });
  });

  it("offers no settings button when it has nowhere to go", () => {
    render(<TransferManagerList />);
    expect(screen.queryByRole("button", { name: "Transfer settings" })).not.toBeInTheDocument();
  });

  it("shows a reconnect wait with its budget and a manual pause action", () => {
    manager.getMaxReconnectAttempts.mockReturnValue(3);
    manager.setJobs([{ ...job("waiting"), status: "reconnecting", reconnectAttempt: 2 }]);
    render(<TransferManagerList />);
    expect(screen.getByText("Waiting to reconnect")).toBeVisible();
    expect(screen.getByText("Reconnect 2/3 (pause to stop)")).toBeVisible();
    expect(screen.getByRole("button", { name: "Pause" })).toBeEnabled();
  });

  it("starts as a compact dock and summarises active work", () => {
    window.localStorage.removeItem("sshc.sftp.queueView");
    manager.setJobs([job("one", { status: "running" })]);
    render(<TransferManagerList />);

    expect(screen.getByRole("button", { name: "Expand Transfer Manager" })).toBeVisible();
    expect(screen.getByText("1 transferring")).toBeVisible();
    expect(screen.getByText("0% · 0 B/s")).toBeVisible();
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("reorders waiting jobs and leaves the ends of the queue anchored", async () => {
    manager.setJobs([job("first"), job("second"), job("third")]);
    render(<TransferManagerList />);

    expect(screen.getByRole("button", { name: "Move first earlier in the queue" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Move third later in the queue" })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Move third earlier in the queue" }));
    expect(manager.move).toHaveBeenCalledWith("third", "up");

    await userEvent.click(screen.getByRole("button", { name: "Move first later in the queue" }));
    expect(manager.move).toHaveBeenCalledWith("first", "down");
  });

  it("offers no reordering for a job that is already running", () => {
    manager.setJobs([job("running", { status: "running" })]);
    render(<TransferManagerList />);

    expect(screen.queryByRole("button", { name: /Move running/ })).not.toBeInTheDocument();
  });

  it("stops the whole queue without touching what is already running", async () => {
    manager.setJobs([job("one")]);
    const { rerender } = render(<TransferManagerList />);

    await userEvent.click(screen.getByRole("button", { name: "Stop starting new transfers" }));
    expect(manager.applySettings).toHaveBeenCalledWith({ processingStopped: true });

    manager.getProcessingStopped.mockReturnValue(true);
    rerender(<TransferManagerList />);
    expect(screen.getByRole("button", { name: "Resume processing the transfer queue" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("Held")).toBeVisible();
  });

  it("remembers the height it was dragged to and the folded state", async () => {
    manager.setJobs([job("one")]);
    const { unmount } = render(<TransferManagerList />);

    const handle = screen.getByRole("separator", { name: "Drag to resize the transfer queue" });
    fireEvent.pointerDown(handle, { clientY: 400 });
    fireEvent.pointerMove(window, { clientY: 340 });
    fireEvent.pointerUp(window);

    expect(screen.getByRole("separator")).toHaveAttribute("aria-valuenow", "284");
    await userEvent.click(screen.getByRole("button", { name: "Collapse Transfer Manager" }));
    unmount();

    render(<TransferManagerList />);
    expect(screen.getByRole("button", { name: "Expand Transfer Manager" })).toBeVisible();
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("opens mobile transfers in a dismissible sheet without restoring the expanded desktop queue", async () => {
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query === mobileViewportQuery, media: query,
      addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }));
    manager.setJobs([job("one")]);
    const navigate = vi.fn();
    const { container, unmount } = render(<TransferManagerList onNavigateLocation={navigate} />);
    try {
      const dock = screen.getByRole("button", { name: "Expand Transfer Manager" });
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(screen.queryByText("one")).not.toBeInTheDocument();
      await userEvent.click(dock);
      expect(screen.getByRole("dialog", { name: "Transfer Manager" })).toBeVisible();
      expect(screen.getByText("one")).toBeVisible();
      expect(container.querySelector('[role="dialog"]')).toBeNull();
      expect(screen.queryByRole("separator")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Close Transfer Manager" })).toHaveFocus();
      await userEvent.click(screen.getByRole("button", { name: "Transfer queue actions" }));
      expect(screen.getByRole("dialog", { name: "Transfer Manager" })).toBeVisible();
      await userEvent.click(screen.getByRole("menuitem", { name: "Pause all" }));
      expect(manager.pauseAll).toHaveBeenCalledOnce();
      expect(screen.getByRole("dialog", { name: "Transfer Manager" })).toBeVisible();
      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(dock).toHaveFocus();
      expect(JSON.parse(window.localStorage.getItem("sshc.sftp.queueView") ?? "{}")).toMatchObject({ collapsed: false, height: 224 });
      await userEvent.click(dock);
      await userEvent.click(screen.getByRole("button", { name: "Transfer settings" }));
      expect(navigate).toHaveBeenCalledWith("/settings/sftp");
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    } finally {
      unmount(); window.matchMedia = originalMatchMedia;
    }
  });

  it("does not render an inert overflow trigger when the queue has no actions", () => {
    render(<TransferManagerList />);
    expect(screen.queryByRole("button", { name: "Transfer queue actions" })).not.toBeInTheDocument();
  });

  it("opens queue actions above the clipped job area and clears failed work", async () => {
    manager.setJobs([job("failed", { status: "failed", allowedActions: ["retry", "cancel", "remove"] })]);
    render(<TransferManagerList />);
    const region = screen.getByRole("region", { name: "Transfer Manager" });
    expect(region).toHaveClass("overflow-visible");
    await userEvent.click(screen.getByRole("button", { name: "Transfer queue actions" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove failed transfers from the list" }));
    expect(manager.clearFailed).toHaveBeenCalledOnce();
  });

  it("removes an individual failed transfer from the list", async () => {
    manager.setJobs([job("failed", { status: "failed", allowedActions: ["retry", "cancel", "remove"] })]);
    render(<TransferManagerList />);
    await userEvent.click(screen.getByRole("button", { name: "Remove from list" }));
    expect(manager.remove).toHaveBeenCalledWith("failed");
  });

  it.each(["remote", "upload", "download"].flatMap((direction) => ["reattach", "paused", "failed"].map((status) => [direction, status])))("asks for a destination check when a %s %s result could not be recorded", (direction, status) => {
    manager.setJobs([job("remote", {
      direction,
      status,
      problem: "sftp_reconciliation_required",
      allowedActions: ["cancel"],
    })]);
    render(<TransferManagerList />);
    expect(screen.getByText("Check the destination")).toBeInTheDocument();
    expect(screen.queryByText("Select the same file")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Resume" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it.each([
    ["a folder copied into itself", "sftp_target_inside_source", "sftp.problem.targetInsideSource"],
    ["a file moved onto itself", "sftp_target_is_source", "sftp.problem.targetIsSource"],
  ] as const)("says why %s failed instead of the general failure", async (_, problem, key) => {
    manager.setJobs([job("remote", {
      direction: "remote",
      status: "failed",
      problem,
      allowedActions: ["retry", "cancel", "remove"],
    })]);
    render(<TransferManagerList />);
    await userEvent.click(screen.getByRole("button", { name: "Show details for remote" }));
    expect(screen.getByText(en[key])).toBeInTheDocument();
    expect(screen.queryByText(en["sftp.problem.failed"])).not.toBeInTheDocument();
  });

  it("keeps an in-flight remote job running even when its details are opened", async () => {
    manager.setJobs([job("remote", {
      direction: "remote",
      status: "running",
      problem: "sftp_reconciliation_required",
      allowedActions: ["pause", "cancel"],
    })]);
    render(<TransferManagerList />);
    expect(screen.queryByText("Check the destination")).not.toBeInTheDocument();
    expect(screen.getByText("Transferring…")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show details for remote" }));
    expect(screen.queryByText(en["sftp.problem.reconciliationRequired"])).not.toBeInTheDocument();
    expect(screen.getByText("Transferring…")).toBeVisible();
  });

  it("keeps paused, reconnecting and attention counts visible while folded", () => {
    window.localStorage.removeItem("sshc.sftp.queueView");
    manager.setJobs([
      job("sending", { status: "running" }),
      job("paused", { status: "paused" }),
      job("reconnect", { status: "reconnecting" }),
      job("overwrite", { status: "needs_overwrite" }),
      job("failed", { status: "failed" }),
    ]);
    render(<TransferManagerList />);
    expect(screen.getByText("1 transferring")).toBeVisible();
    expect(screen.getByText("1 paused")).toBeVisible();
    expect(screen.getByText("1 reconnecting")).toBeVisible();
    expect(screen.getByText("Needs attention: 1")).toBeVisible();
    expect(screen.getByText("1 failed")).toBeVisible();
    expect(screen.getByRole("button", { name: "Expand Transfer Manager" })).toHaveAccessibleDescription(/Needs attention: 1.*1 failed.*1 transferring.*1 reconnecting.*1 paused/);
    expect(screen.queryByText(/5 transferring/)).not.toBeInTheDocument();
  });

  it("does not describe stopped processing or a missing upload source as transferring", () => {
    window.localStorage.removeItem("sshc.sftp.queueView");
    manager.getProcessingStopped.mockReturnValue(true);
    manager.hasUploadSource.mockReturnValue(false);
    manager.setJobs([job("queued"), job("upload", { direction: "upload" })]);
    render(<TransferManagerList />);
    expect(screen.getByText("1 held")).toBeVisible();
    expect(screen.getByText("Needs attention: 1")).toBeVisible();
    expect(screen.queryByText(/transferring/)).not.toBeInTheDocument();
  });

  it("keeps a failed-only queue distinguishable from finished work", () => {
    window.localStorage.removeItem("sshc.sftp.queueView");
    manager.setJobs([job("failed", { status: "failed" })]);
    render(<TransferManagerList />);
    expect(screen.getByText("1 failed")).toBeVisible();
    expect(screen.queryByText("1 transfers")).not.toBeInTheDocument();
  });

  it.each([
    [{ direction: "remote", operation: "copy", sourceAlias: "edge", sourcePath: "/srv/report.txt", alias: "nas", remotePath: "/backup/report.txt" }, "Copy", "edge:/srv/report.txt → nas:/backup/report.txt"],
    [{ direction: "remote", operation: "move", sourceAlias: "edge", sourcePath: "/old/report.txt", alias: "edge", remotePath: "/new/report.txt" }, "Move", "edge:/old/report.txt → edge:/new/report.txt"],
    [{ direction: "remote", operation: "get", sourceAlias: "edge", sourcePath: "/srv/report.txt", alias: "edge", remotePath: "/home/alice/report.txt" }, "Download", "edge:/srv/report.txt → Local:/home/alice/report.txt"],
    [{ direction: "remote", operation: "put", sourceAlias: "edge", sourcePath: "/home/alice/report.txt", alias: "edge", remotePath: "/srv/report.txt" }, "Upload", "Local:/home/alice/report.txt → edge:/srv/report.txt"],
    [{ direction: "download", alias: "edge", remotePath: "/srv/report.txt" }, "Download", "edge:/srv/report.txt → Browser save location"],
    [{ direction: "upload", alias: "edge", remotePath: "/srv/report.txt" }, "Upload", "Browser:report.txt → edge:/srv/report.txt"],
    [{ direction: "remote", operation: "delete", sourceAlias: "edge", sourcePath: "/old/report.txt", alias: "edge", remotePath: "/old/report.txt" }, "Delete", "edge:/old/report.txt"],
  ] as const)("shows the actual source and destination for %j", async (overrides, operation, route) => {
    manager.setJobs([job("report.txt", overrides)]);
    render(<TransferManagerList />);
    expect(screen.getByText(operation, { selector: "span.mr-2" })).toBeVisible();
    expect(screen.queryByText(route)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show details for report.txt" }));
    expect(screen.getByText(route)).toBeVisible();
  });
});

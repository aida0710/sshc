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
    getMaxConcurrent: vi.fn(() => 2),
    getClearCompletedAfter: vi.fn(() => 0),
    getProcessingStopped: vi.fn(() => false),
    getSpeedLimitBytesPerSecond: vi.fn(() => 0),
    getAutoReconnect: vi.fn(() => false),
    getMaxReconnectAttempts: vi.fn(() => 0),
    getLargeFileThreshold: vi.fn(() => 100 << 20),
    getLargeFileParallelism: vi.fn(() => 4),
    getLargeFileChunkBytes: vi.fn(() => 32 << 20),
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
    manager.getMaxConcurrent.mockReturnValue(2);
    manager.getClearCompletedAfter.mockReturnValue(0);
    manager.getProcessingStopped.mockReturnValue(false);
    manager.getSpeedLimitBytesPerSecond.mockReturnValue(0);
    manager.getAutoReconnect.mockReturnValue(false);
    manager.getMaxReconnectAttempts.mockReturnValue(0);
    manager.getLargeFileThreshold.mockReturnValue(100 << 20);
    manager.getLargeFileParallelism.mockReturnValue(4);
    manager.getLargeFileChunkBytes.mockReturnValue(32 << 20);
    manager.hasUploadSource.mockReturnValue(true);
    manager.setJobs([]);
  });

  it("opens the saved settings only when requested before a transfer starts", async () => {
    render(<TransferManagerList />);
    expect(screen.getByRole("button", { name: "Collapse Transfer Manager" })).toBeVisible();
    expect(screen.getByLabelText("Split at")).not.toBeVisible();
    const disclosure = screen.getByText("Transfer settings");
    disclosure.focus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("spinbutton", { name: "Split at" })).toHaveValue(100);
    expect(screen.getByRole("spinbutton", { name: "Streams" })).toHaveValue(4);
    expect(screen.getByRole("spinbutton", { name: "Chunk" })).toHaveValue(32);
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("commits an explicit KiB/s limit and enables a bounded recovery budget", async () => {
    render(<TransferManagerList />);
    await userEvent.click(screen.getByText("Transfer settings"));
    const speed = screen.getByRole("spinbutton", { name: "Speed limit" });
    fireEvent.change(speed, { target: { value: "2048" } });
    fireEvent.blur(speed);
    expect(manager.applySettings).toHaveBeenLastCalledWith(expect.objectContaining({ speedLimitBytesPerSecond: 2048 * 1024 }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Recover after connection loss" }));
    expect(manager.applySettings).toHaveBeenLastCalledWith(expect.objectContaining({ autoReconnect: true, maxReconnectAttempts: 3 }));
    expect(screen.getByText("KiB/s")).toBeVisible();
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

  it("sends the concurrency and auto-clear settings to the engine together", async () => {
    manager.getClearCompletedAfter.mockReturnValue(300);
    manager.setJobs([job("one")]);
    render(<TransferManagerList />);
    await userEvent.click(screen.getByText("Transfer settings"));

    expect(screen.getByRole("combobox", { name: "Clear finished after" })).toHaveValue("300");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Concurrent transfers" }), "5");
    expect(manager.applySettings).toHaveBeenCalledWith({ maxConcurrent: 5, clearCompletedAfterSeconds: 300, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Clear finished after" }), "0");
    expect(manager.applySettings).toHaveBeenLastCalledWith({ maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });

    const splitAt = screen.getByRole("spinbutton", { name: "Split at" });
    fireEvent.change(splitAt, { target: { value: "73" } });
    fireEvent.blur(splitAt);
    expect(manager.applySettings).toHaveBeenLastCalledWith({ maxConcurrent: 2, clearCompletedAfterSeconds: 300, processingStopped: false, largeFileThresholdBytes: 73 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });

    const streams = screen.getByRole("spinbutton", { name: "Streams" });
    fireEvent.change(streams, { target: { value: "128" } });
    fireEvent.blur(streams);
    expect(manager.applySettings).toHaveBeenLastCalledWith({ maxConcurrent: 2, clearCompletedAfterSeconds: 300, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 128, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });

    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.change(chunk, { target: { value: "41" } });
    fireEvent.blur(chunk);
    expect(manager.applySettings).toHaveBeenLastCalledWith({ maxConcurrent: 2, clearCompletedAfterSeconds: 300, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 41 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });
  });

  it("stops the whole queue without touching what is already running", async () => {
    manager.setJobs([job("one")]);
    const { rerender } = render(<TransferManagerList />);

    await userEvent.click(screen.getByRole("button", { name: "Stop starting new transfers" }));
    expect(manager.applySettings).toHaveBeenCalledWith({ maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: true, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0 });

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
    const { container, unmount } = render(<TransferManagerList />);
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
      expect(screen.getByLabelText("Split at")).not.toBeVisible();
      await userEvent.click(screen.getByText("Transfer settings"));
      expect(screen.getByRole("spinbutton", { name: "Split at" })).toBeVisible();
      await userEvent.click(screen.getByRole("button", { name: "Transfer queue actions" }));
      expect(screen.getByRole("dialog", { name: "Transfer Manager" })).toBeVisible();
      await userEvent.click(screen.getByRole("menuitem", { name: "Pause all" }));
      expect(manager.pauseAll).toHaveBeenCalledOnce();
      expect(screen.getByRole("dialog", { name: "Transfer Manager" })).toBeVisible();
      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(dock).toHaveFocus();
      expect(JSON.parse(window.localStorage.getItem("sshc.sftp.queueView") ?? "{}")).toMatchObject({ collapsed: false, height: 224 });
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
  ] as const)("says why %s failed instead of the general failure", (_, problem, key) => {
    manager.setJobs([job("remote", {
      direction: "remote",
      status: "failed",
      problem,
      allowedActions: ["retry", "cancel", "remove"],
    })]);
    render(<TransferManagerList />);
    expect(screen.getByText(en[key])).toBeInTheDocument();
    expect(screen.queryByText(en["sftp.problem.failed"])).not.toBeInTheDocument();
  });

  it("shows a remote job whose operation is in flight as running", () => {
    manager.setJobs([job("remote", {
      direction: "remote",
      status: "running",
      problem: "sftp_reconciliation_required",
      allowedActions: ["pause", "cancel"],
    })]);
    render(<TransferManagerList />);
    expect(screen.queryByText("Check the destination")).not.toBeInTheDocument();
    expect(screen.getByText("Transferring…")).toBeInTheDocument();
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
  ] as const)("shows the actual source and destination for %j", (overrides, operation, route) => {
    manager.setJobs([job("report.txt", overrides)]);
    render(<TransferManagerList />);
    expect(screen.getByText(operation, { selector: "span.mr-2" })).toBeVisible();
    expect(screen.getByText(route)).toBeVisible();
  });
});

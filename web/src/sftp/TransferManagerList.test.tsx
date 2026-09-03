import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TransferManagerList } from "./TransferManagerList";

type Job = {
  id: string;
  status: string;
  allowedActions: string[];
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
    attempt: 1,
    problem: "",
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
    manager.getLargeFileThreshold.mockReturnValue(100 << 20);
    manager.getLargeFileParallelism.mockReturnValue(4);
    manager.getLargeFileChunkBytes.mockReturnValue(32 << 20);
    manager.setJobs([]);
  });

  it("keeps the saved split settings available before a transfer starts", () => {
    render(<TransferManagerList />);
    expect(screen.getByRole("button", { name: "Collapse Transfer Manager" })).toBeVisible();
    expect(screen.getByRole("spinbutton", { name: "Split at" })).toHaveValue(100);
    expect(screen.getByRole("spinbutton", { name: "Streams" })).toHaveValue(4);
    expect(screen.getByRole("spinbutton", { name: "Chunk" })).toHaveValue(32);
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("starts as a compact dock and summarises active work", () => {
    window.localStorage.removeItem("sshc.sftp.queueView");
    manager.setJobs([job("one", { status: "running" })]);
    render(<TransferManagerList />);

    expect(screen.getByRole("button", { name: "Expand Transfer Manager" })).toBeVisible();
    expect(screen.getByText("1 active · 0% · 0 B/s")).toBeVisible();
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

    expect(screen.getByRole("combobox", { name: "Clear finished after" })).toHaveValue("300");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "At once" }), "5");
    expect(manager.applySettings).toHaveBeenCalledWith(5, 300, false, 100 << 20, 4, 32 << 20);

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Clear finished after" }), "0");
    expect(manager.applySettings).toHaveBeenLastCalledWith(2, 0, false, 100 << 20, 4, 32 << 20);

    const splitAt = screen.getByRole("spinbutton", { name: "Split at" });
    fireEvent.change(splitAt, { target: { value: "73" } });
    fireEvent.blur(splitAt);
    expect(manager.applySettings).toHaveBeenLastCalledWith(2, 300, false, 73 << 20, 4, 32 << 20);

    const streams = screen.getByRole("spinbutton", { name: "Streams" });
    fireEvent.change(streams, { target: { value: "128" } });
    fireEvent.blur(streams);
    expect(manager.applySettings).toHaveBeenLastCalledWith(2, 300, false, 100 << 20, 128, 32 << 20);

    const chunk = screen.getByRole("spinbutton", { name: "Chunk" });
    fireEvent.change(chunk, { target: { value: "41" } });
    fireEvent.blur(chunk);
    expect(manager.applySettings).toHaveBeenLastCalledWith(2, 300, false, 100 << 20, 4, 41 << 20);
  });

  it("stops the whole queue without touching what is already running", async () => {
    manager.setJobs([job("one")]);
    const { rerender } = render(<TransferManagerList />);

    await userEvent.click(screen.getByRole("button", { name: "Stop starting new transfers" }));
    expect(manager.applySettings).toHaveBeenCalledWith(2, 0, true, 100 << 20, 4, 32 << 20);

    manager.getProcessingStopped.mockReturnValue(true);
    rerender(<TransferManagerList />);
    expect(screen.getByRole("button", { name: "Start transfers again" })).toHaveAttribute("aria-pressed", "true");
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

  it("shows a touch-sized grip and snaps mobile resizing without changing the desktop height", () => {
    const originalMatchMedia = window.matchMedia;
    const originalInnerHeight = window.innerHeight;
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
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 640 });
    manager.setJobs([job("one")]);

    const { unmount } = render(<TransferManagerList />);
    try {
      const handle = screen.getByRole("separator", { name: "Drag to resize the transfer queue" });
      expect(handle).toHaveClass("h-6", "touch-none");
      expect(handle.querySelector("span")).toHaveClass("w-10", "rounded-full");

      fireEvent.pointerDown(handle, { clientY: 400 });
      fireEvent.pointerMove(window, { clientY: 300 });
      fireEvent.pointerUp(window);

      expect(handle).toHaveAttribute("aria-valuenow", "360");
      expect(JSON.parse(window.localStorage.getItem("sshc.sftp.queueView") ?? "{}")).toMatchObject({
        height: 224,
        mobileHeight: 360,
      });
    } finally {
      unmount();
      window.matchMedia = originalMatchMedia;
      Object.defineProperty(window, "innerHeight", { configurable: true, value: originalInnerHeight });
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
    await userEvent.click(screen.getByRole("menuitem", { name: "Remove failed from list" }));
    expect(manager.clearFailed).toHaveBeenCalledOnce();
  });

  it("removes an individual failed transfer from the list", async () => {
    manager.setJobs([job("failed", { status: "failed", allowedActions: ["retry", "cancel", "remove"] })]);
    render(<TransferManagerList />);
    await userEvent.click(screen.getByRole("button", { name: "Remove from list" }));
    expect(manager.remove).toHaveBeenCalledWith("failed");
  });
});

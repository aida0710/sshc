import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreateTransferJob, StreamDownloadOptions, TransferJob, TransferJobAction } from "./api";
import { SFTPTransferManager } from "./transferManager";

function serverAPI(overrides: Record<string, unknown> = {}) {
  const jobs = new Map<string, TransferJob>();
  const createTransfer = vi.fn(async (input: CreateTransferJob): Promise<TransferJob> => {
    const existing = jobs.get(input.id);
    if (existing !== undefined) return existing;
    const now = new Date().toISOString();
    const job: TransferJob = {
      ...input, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1,
      status: "queued", attempt: 1, problem: "", createdAt: now, updatedAt: now,
    };
    jobs.set(job.id, job);
    return job;
  });
  const updateTransfer = vi.fn(async (id: string, action: TransferJobAction, options: { transferredBytes?: number; problem?: string; resetProgress?: boolean } = {}): Promise<TransferJob> => {
    const current = jobs.get(id);
    if (current === undefined) throw new Error("not_found");
    const statuses: Partial<Record<TransferJobAction, TransferJob["status"]>> = {
      start: "running", pause: "paused", resume: "queued", retry: "queued", cancel: "cancelled",
      complete: "completed", fail: "failed", needs_overwrite: "needs_overwrite",
    };
    const updated: TransferJob = {
      ...current,
      ...(options.transferredBytes === undefined ? {} : { transferredBytes: options.transferredBytes }),
      ...(options.resetProgress ? { transferredBytes: 0 } : {}),
      status: statuses[action] ?? current.status,
      attempt: action === "retry" ? current.attempt + 1 : current.attempt,
      problem: action === "fail" ? options.problem ?? "sftp_failed" : action === "retry" ? "" : current.problem,
      updatedAt: new Date().toISOString(),
    };
    jobs.set(id, updated);
    return updated;
  });
  return {
    listTransfers: vi.fn(async () => ({ maxConcurrent: 2, jobs: [...jobs.values()] })),
    createTransfer,
    updateTransfer,
    startUpload: vi.fn(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "absent" })),
    appendUpload: vi.fn(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({ id, path, offset: total, size: total, expectedRevision: "" })),
    completeUpload: vi.fn(async () => undefined),
    cancelUpload: vi.fn(async () => undefined),
    streamDownload: vi.fn(async (_alias: string, _path: string, _directory: boolean, _offset: number, _options: StreamDownloadOptions) => ({ bytes: 0, total: 0 })),
    saveDownload: vi.fn(() => undefined),
    ...overrides,
  };
}

describe("SFTPTransferManager", () => {
  beforeEach(() => localStorage.clear());

  it("does not create persistent storage for an empty queue", async () => {
    const manager = new SFTPTransferManager(serverAPI());
    await manager.reconcile();
    expect(localStorage.getItem("sshc.sftp.transfer-manager.v2")).toBeNull();
  });

  it("resumes a failed file download with Range state and reports speed and completion", async () => {
    let now = 0;
    let calls = 0;
    const api = serverAPI();
    api.streamDownload.mockImplementation(async (_alias, _path, _directory, offset, options) => {
      calls += 1;
      if (calls === 1) {
        expect(offset).toBe(0);
        now = 1_000;
        options.onChunk(new Uint8Array(1 << 20), 2 << 20);
        throw new Error("connection_lost");
      }
      expect(offset).toBe(1 << 20);
      now = 2_000;
      options.onChunk(new Uint8Array(1 << 20), 2 << 20);
      return { bytes: 2 << 20, total: 2 << 20 };
    });
    const manager = new SFTPTransferManager(api, 2, () => now);
    manager.addDownload("edge", "/remote/large.bin", "file", 2 << 20);

    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    const job = manager.getSnapshot()[0]!;
    expect(job.transferredBytes).toBe(2 << 20);
    expect(job.bytesPerSecond).toBeGreaterThan(0);
    expect(job.remainingSeconds).toBe(0);
    expect(api.streamDownload).toHaveBeenCalledTimes(2);
    expect(api.saveDownload).toHaveBeenCalledOnce();
    expect(manager.getNoticeSnapshot()[0]).toMatchObject({ status: "completed", name: "large.bin" });
  });

  it("retries only failed files in a folder batch", async () => {
    let failBad = true;
    const api = serverAPI();
    api.startUpload.mockImplementation(async (_alias, id, path, size) => {
      if (path.endsWith("bad.txt") && failBad) throw new Error("connection_lost");
      return { id, path, offset: 0, size, expectedRevision: "absent" };
    });
    const manager = new SFTPTransferManager(api);
    manager.addUploads([
      { alias: "edge", remotePath: "/project/good.txt", localName: "project/good.txt", file: new File(["good"], "good.txt") },
      { alias: "edge", remotePath: "/project/bad.txt", localName: "project/bad.txt", file: new File(["bad"], "bad.txt") },
    ], { id: "batch_project1", name: "project", kind: "folder" });

    await vi.waitFor(() => expect(manager.getSnapshot().map((job) => job.status).sort()).toEqual(["completed", "failed"]));
    expect(manager.getNoticeSnapshot()).toContainEqual(expect.objectContaining({ status: "failed", name: "project/bad.txt" }));
    const goodCalls = () => api.startUpload.mock.calls.filter((call) => call[2].endsWith("good.txt")).length;
    expect(goodCalls()).toBe(1);
    failBad = false;
    manager.retryFailed("batch_project1");
    await vi.waitFor(() => expect(manager.getSnapshot().every((job) => job.status === "completed")).toBe(true));
    expect(goodCalls()).toBe(1);
    expect(manager.getSnapshot().find((job) => job.name.endsWith("bad.txt"))?.attempt).toBe(2);
  });

  it("applies one concurrency limit to uploads and downloads", async () => {
    const uploadReleases: Array<() => void> = [];
    let releaseDownload: (() => void) | undefined;
    const api = serverAPI();
    api.appendUpload.mockImplementation(async (_alias, id, path, _offset, total) => {
      await new Promise<void>((resolve) => uploadReleases.push(resolve));
      return { id, path, offset: total, size: total, expectedRevision: "" };
    });
    api.streamDownload.mockImplementation(async (_alias, _path, _directory, offset, options) => {
      await new Promise<void>((resolve) => { releaseDownload = resolve; });
      options.onChunk(new Uint8Array(4), 4);
      return { bytes: offset + 4, total: 4 };
    });
    const manager = new SFTPTransferManager(api, 2);
    manager.addDownload("edge", "/download.bin", "file", 4);
    manager.addUploads([1, 2].map((number) => ({
      alias: "edge", remotePath: `/file-${number}.bin`, localName: `file-${number}.bin`, file: new File([String(number)], `file-${number}.bin`),
    })), { id: "batch_limit001", name: "two files", kind: "folder" });

    await vi.waitFor(() => expect(api.streamDownload).toHaveBeenCalledOnce());
    await vi.waitFor(() => expect(api.appendUpload).toHaveBeenCalledOnce());
    expect(manager.getSnapshot().filter((job) => job.status === "running")).toHaveLength(2);
    expect(manager.getSnapshot().filter((job) => job.status === "queued")).toHaveLength(1);
    releaseDownload?.();
    await vi.waitFor(() => expect(api.appendUpload).toHaveBeenCalledTimes(2));
    while (uploadReleases.length > 0) uploadReleases.shift()?.();
    await vi.waitFor(() => expect(manager.getSnapshot().every((job) => job.status === "completed")).toBe(true));
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreateTransferJob, StreamDownloadOptions, TransferJob, TransferJobAction } from "./api";
import { SFTPTransferManager } from "./transferManager";

function engineAPI(overrides: Record<string, unknown> = {}) {
  const jobs = new Map<string, TransferJob>();
  const now = () => new Date().toISOString();
  const allowedActions = (status: TransferJob["status"]): TransferJob["allowedActions"] => {
    if (status === "queued" || status === "running") return ["pause", "cancel"];
    if (status === "paused" || status === "reattach" || status === "needs_overwrite") return ["resume", "cancel"];
    if (status === "failed") return ["retry", "cancel"];
    return [];
  };
  const createTransfer = vi.fn(async (input: CreateTransferJob): Promise<TransferJob> => {
    const existing = jobs.get(input.id);
    if (existing !== undefined) return existing;
    const job: TransferJob = {
      ...input, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1, status: "queued", allowedActions: ["pause", "cancel"],
      attempt: 1, problem: "", expectedRevision: "", sourceFingerprint: "", overwrite: false,
      downloadRevision: "", createdAt: now(), updatedAt: now(),
    };
    jobs.set(job.id, job);
    return job;
  });
  const updateTransfer = vi.fn(async (
    id: string,
    action: TransferJobAction,
    options: { transferredBytes?: number; problem?: string; resetProgress?: boolean } = {},
  ): Promise<TransferJob> => {
    const current = jobs.get(id);
    if (current === undefined) throw new Error("sftp_transfer_not_found");
    const statuses: Partial<Record<TransferJobAction, TransferJob["status"]>> = {
      start: "running", pause: "paused", resume: "queued", retry: "queued", cancel: "cancelled",
      complete: "completed", fail: "failed", needs_overwrite: "needs_overwrite",
    };
    const status = statuses[action] ?? current.status;
    const updated: TransferJob = {
      ...current, status, allowedActions: allowedActions(status),
      attempt: action === "retry" ? current.attempt + 1 : current.attempt,
      problem: action === "fail" ? options.problem ?? "sftp_failed" : action === "retry" ? "" : current.problem,
      overwrite: action === "resume" && current.status === "needs_overwrite" ? true : current.overwrite,
      ...(options.resetProgress ? { transferredBytes: 0, downloadRevision: "" } : {}),
      ...(options.transferredBytes === undefined ? {} : { transferredBytes: options.transferredBytes }),
      updatedAt: now(),
    };
    jobs.set(id, updated);
    return updated;
  });
  return {
    jobs,
    listTransfers: vi.fn(async () => ({ maxConcurrent: 2, jobs: [...jobs.values()] })),
    createTransfer,
    updateTransfer,
    clearFinishedTransfers: vi.fn(async () => {
      for (const [id, job] of jobs) if (job.status === "completed" || job.status === "cancelled") jobs.delete(id);
    }),
    checkpointDownload: vi.fn(async (id: string, offset: number, revision: string) => {
      const current = jobs.get(id);
      if (current === undefined) throw new Error("sftp_transfer_not_found");
      const updated = { ...current, transferredBytes: offset, downloadRevision: revision };
      jobs.set(id, updated);
      return updated;
    }),
    verifyDownload: vi.fn(async () => undefined),
    startUpload: vi.fn(async (_alias: string, id: string, path: string, size: number) => ({
      id, path, offset: 0, size, expectedRevision: "absent",
    })),
    appendUpload: vi.fn(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({
      id, path, offset: total, size: total, expectedRevision: "",
    })),
    completeUpload: vi.fn(async (_alias: string, id: string, _path: string, size: number) => {
      const current = jobs.get(id);
      if (current !== undefined) jobs.set(id, { ...current, status: "completed", allowedActions: [], transferredBytes: size, remainingSeconds: 0 });
    }),
    cancelUpload: vi.fn(async (_alias: string, id: string) => {
      const current = jobs.get(id);
      if (current !== undefined) jobs.set(id, { ...current, status: "cancelled", allowedActions: [], problem: "" });
    }),
    streamDownload: vi.fn(async (
      _alias: string, _id: string, _path: string, _directory: boolean, _offset: number,
      options: StreamDownloadOptions,
    ) => {
      options.onRevision?.('"revision"');
      await options.onChunk(new TextEncoder().encode("data"), 4);
      return { bytes: 4, total: 4 };
    }),
    saveDownload: vi.fn((_remotePath: string, _directory: boolean, _parts: BlobPart[]) => undefined),
    ...overrides,
  };
}

describe("SFTPTransferManager engine ownership", () => {
  beforeEach(() => {
    localStorage.clear();
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: undefined });
  });

  it("uses the engine list as the exact transfer snapshot and ignores browser storage", async () => {
    localStorage.setItem("sshc.sftp.transfer-manager.v3", JSON.stringify([{ id: "browser-only" }]));
    const api = engineAPI();
    await api.createTransfer({
      id: "transfer_engine1", batchId: "batch_engine001", batchName: "remote.bin", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "remote.bin", remotePath: "/remote.bin",
      totalBytes: 4, lastModified: 0,
    });
    const manager = new SFTPTransferManager(api, 0);
    await manager.reconcile();
    expect(manager.getSnapshot().map((job) => job.id)).toEqual(["transfer_engine1"]);
    expect(localStorage.getItem("sshc.sftp.transfer-manager.v3")).toContain("browser-only");
  });

  it("registers an upload in the engine before starting its data plane", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api);
    await manager.addUploads([{
      alias: "edge", remotePath: "/file.txt", localName: "file.txt", file: new File(["data"], "file.txt"),
    }]);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.createTransfer.mock.invocationCallOrder[0]).toBeLessThan(api.startUpload.mock.invocationCallOrder[0]!);
    expect(api.createTransfer).toHaveBeenCalledWith(expect.objectContaining({
      batchName: "file.txt", batchKind: "file", lastModified: expect.any(Number),
    }));
  });

  it("registers downloads in the engine and mirrors engine progress", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api);
    await manager.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.createTransfer.mock.invocationCallOrder[0]).toBeLessThan(api.updateTransfer.mock.invocationCallOrder[0]!);
    expect(api.checkpointDownload).toHaveBeenCalledWith(expect.any(String), 4, '"revision"');
    expect(api.saveDownload).toHaveBeenCalledOnce();
  });

  it("resumes a file download after a transient disconnect", async () => {
    let calls = 0;
    const api = engineAPI();
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, offset, options) => {
      options.onRevision?.('"revision-resume"');
      calls += 1;
      if (calls === 1) {
        expect(offset).toBe(0);
        await options.onChunk(new TextEncoder().encode("abc"), 6);
        throw new Error("connection_lost");
      }
      expect(offset).toBe(3);
      await options.onChunk(new TextEncoder().encode("def"), 6);
      return { bytes: 6, total: 6 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addDownload("edge", "/resume.bin", "file", 6);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.streamDownload).toHaveBeenCalledTimes(2);
    const parts = api.saveDownload.mock.calls[0]![2];
    await expect(new Blob(parts).text()).resolves.toBe("abcdef");
  });

  it("retries only failed uploads in an engine-owned folder batch", async () => {
    let failBad = true;
    const api = engineAPI();
    api.startUpload.mockImplementation(async (_alias, id, path, size) => {
      if (path.endsWith("bad.txt") && failBad) throw new Error("connection_lost");
      return { id, path, offset: 0, size, expectedRevision: "absent" };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addUploads([
      { alias: "edge", remotePath: "/project/good.txt", localName: "project/good.txt", file: new File(["good"], "good.txt") },
      { alias: "edge", remotePath: "/project/bad.txt", localName: "project/bad.txt", file: new File(["bad"], "bad.txt") },
    ], { id: "batch_project1", name: "project", kind: "folder" });
    await vi.waitFor(() => expect(manager.getSnapshot().map((job) => job.status).sort()).toEqual(["completed", "failed"]));
    const goodCalls = () => api.startUpload.mock.calls.filter((call) => call[2].endsWith("good.txt")).length;
    expect(goodCalls()).toBe(1);
    failBad = false;
    await manager.retryFailed("batch_project1");
    await vi.waitFor(() => expect(manager.getSnapshot().every((job) => job.status === "completed")).toBe(true));
    expect(goodCalls()).toBe(1);
    expect(manager.getSnapshot().find((job) => job.name.endsWith("bad.txt"))?.attempt).toBe(2);
  });

  it("uses the engine concurrency limit for uploads and downloads together", async () => {
    const releases: Array<() => void> = [];
    const api = engineAPI();
    api.appendUpload.mockImplementation(async (_alias, id, path, _offset, total) => {
      await new Promise<void>((resolve) => releases.push(resolve));
      return { id, path, offset: total, size: total, expectedRevision: "" };
    });
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, _offset, options) => {
      options.onRevision?.('"revision-limit"');
      await new Promise<void>((resolve) => releases.push(resolve));
      await options.onChunk(new Uint8Array(4), 4);
      return { bytes: 4, total: 4 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.addDownload("edge", "/download.bin", "file", 4);
    await manager.addUploads([1, 2].map((number) => ({
      alias: "edge", remotePath: `/file-${number}.bin`, localName: `file-${number}.bin`,
      file: new File([String(number)], `file-${number}.bin`),
    })), { id: "batch_limit001", name: "two files", kind: "folder" });
    await vi.waitFor(() => expect(manager.getSnapshot().filter((job) => job.status === "running")).toHaveLength(2));
    expect(manager.getSnapshot().filter((job) => job.status === "queued")).toHaveLength(1);
    while (releases.length > 0) releases.shift()?.();
    await vi.waitFor(() => expect(releases.length).toBeGreaterThan(0));
    while (releases.length > 0) releases.shift()?.();
    await vi.waitFor(() => expect(manager.getSnapshot().every((job) => job.status === "completed")).toBe(true));
  });

  it("applies pause, resume, cancel, and clear through engine APIs", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api, 0);
    const id = await manager.addDownload("edge", "/queued.bin", "file", 4);
    await manager.pause(id);
    expect(manager.getSnapshot()[0]?.status).toBe("paused");
    await manager.resume(id);
    expect(manager.getSnapshot()[0]?.status).toBe("queued");
    await manager.cancel(id);
    expect(manager.getSnapshot()[0]?.status).toBe("cancelled");
    await manager.clearFinished();
    expect(manager.getSnapshot()).toEqual([]);
    expect(api.clearFinishedTransfers).toHaveBeenCalledOnce();
  });

  it("replaces a stale browser cache when another WebView changes the engine", async () => {
    const api = engineAPI();
    const first = new SFTPTransferManager(api, 0);
    const second = new SFTPTransferManager(api, 0);
    const id = await first.addDownload("edge", "/shared.bin", "file", 4);
    await second.reconcile();
    await second.pause(id);
    await first.reconcile();
    expect(first.getSnapshot()[0]?.status).toBe("paused");
  });

  it("reattaches a matching browser File to an existing engine upload job", async () => {
    const api = engineAPI();
    const file = new File(["data"], "file.txt", { lastModified: 123 });
    const first = new SFTPTransferManager(api, 0);
    await first.addUploads([{ alias: "edge", remotePath: "/file.txt", localName: "file.txt", file }]);
    const reloaded = new SFTPTransferManager(api);
    await reloaded.reconcile();
    expect(reloaded.hasUploadSource(reloaded.getSnapshot()[0]!.id)).toBe(false);
    await reloaded.addUploads([{ alias: "edge", remotePath: "/file.txt", localName: "file.txt", file }]);
    await vi.waitFor(() => expect(reloaded.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.createTransfer).toHaveBeenCalledOnce();
  });

  it("takes over an engine upload left running by a reloaded WebView", async () => {
    const api = engineAPI();
    const file = new File(["data"], "running.txt", { lastModified: 456 });
    const created = await api.createTransfer({
      id: "transfer_running1", batchId: "batch_running01", batchName: "running.txt", batchKind: "file",
      alias: "edge", direction: "upload", kind: "file", name: "running.txt", remotePath: "/running.txt",
      totalBytes: file.size, lastModified: file.lastModified,
    });
    await api.updateTransfer(created.id, "start");
    const reloaded = new SFTPTransferManager(api);
    await reloaded.reconcile();
    await reloaded.addUploads([{ alias: "edge", remotePath: "/running.txt", localName: "running.txt", file }]);
    await vi.waitFor(() => expect(reloaded.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.updateTransfer).toHaveBeenCalledWith(created.id, "pause");
    expect(api.createTransfer).toHaveBeenCalledOnce();
  });

  it("resumes a durable OPFS download from engine revision and offset state", async () => {
    const body = new Uint8Array([1, 2, 3, 4]);
    const writer = {
      truncate: vi.fn(async () => undefined), seek: vi.fn(async () => undefined),
      write: vi.fn(async () => undefined), close: vi.fn(async () => undefined),
    };
    const handle = {
      createWritable: vi.fn(async () => writer),
      getFile: vi.fn(async () => new File([body], "part")),
    };
    const root = {
      async *values() { /* no orphan entries */ },
      getFileHandle: vi.fn(async () => handle), removeEntry: vi.fn(async () => undefined),
    };
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: vi.fn(async () => root) } });
    const api = engineAPI();
    const created = await api.createTransfer({
      id: "transfer_fullopfs", batchId: "batch_fullopfs", batchName: "full.bin", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "full.bin", remotePath: "/full.bin",
      totalBytes: 4, lastModified: 0,
    });
    api.jobs.set(created.id, {
      ...created, transferredBytes: 4, downloadRevision: '"content-sha256:full"',
      status: "failed", allowedActions: ["retry", "cancel"], problem: "connection_lost",
    });
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.retry(created.id);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.verifyDownload).toHaveBeenCalledWith("edge", created.id, "/full.bin", '"content-sha256:full"');
    expect(api.streamDownload).not.toHaveBeenCalled();
    expect(api.saveDownload).toHaveBeenCalledOnce();
  });

  it("does not admit more than 200 cached engine jobs", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api, 0);
    for (let index = 0; index < 200; index += 1) {
      await manager.addDownload("edge", `/file-${index}.bin`, "file", 1);
    }
    await expect(manager.addDownload("edge", "/overflow.bin", "file", 1)).rejects.toThrow("sftp_transfer_limit");
    expect(manager.getSnapshot()).toHaveLength(200);
  });
});

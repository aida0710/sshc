import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  BrowserSave,
  CreateTransferJob,
  DownloadStream,
  ResumableUpload,
  StreamDownloadOptions,
  TransferJob,
  TransferJobAction,
  TransferSettings,
  UploadChunk,
  UploadCompletion,
  UploadStart,
  UploadTarget,
} from "./api";
import { browserSaveRetentionMs } from "./downloadPlane";
import { SFTPTransferManager } from "./transferManager";

// Exercise the same waiting transition without a production one-second backoff.
const testReconnectDelayMs = 10;

function engineAPI(overrides: Record<string, unknown> = {}) {
  const jobs = new Map<string, TransferJob>();
  const now = () => new Date().toISOString();
  const allowedActions = (status: TransferJob["status"], problem = ""): TransferJob["allowedActions"] => {
    if (problem === "sftp_reconciliation_required" && status !== "running" && status !== "reconnecting") return ["cancel"];
    if (status === "queued" || status === "running" || status === "reconnecting") return ["pause", "cancel"];
    if (status === "paused" || status === "reattach" || status === "needs_overwrite") return ["resume", "cancel"];
    if (status === "failed") return ["retry", "cancel", "remove"];
    if (status === "completed" || status === "cancelled") return ["remove"];
    return [];
  };
  const recoverySettings = { autoReconnect: false, maxReconnectAttempts: 0 };
  const createTransfer = vi.fn(async (input: CreateTransferJob): Promise<TransferJob> => {
    const existing = jobs.get(input.id);
    if (existing !== undefined) return existing;
    const job: TransferJob = {
      sourceAlias: "", sourcePath: "", operation: "", ...input,
      transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1, status: "queued", allowedActions: ["pause", "cancel"],
      attempt: 1, reconnectAttempt: 0, reconnectAt: "", problem: "", expectedRevision: "", sourceFingerprint: "", overwrite: false,
      downloadRevision: "", downloadParts: [], createdAt: now(), updatedAt: now(),
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
      start: "running", reconnect: "reconnecting", pause: "paused", resume: "queued", retry: "queued", cancel: "cancelled",
      complete: "completed", fail: "failed", needs_overwrite: "needs_overwrite",
    };
    const status = statuses[action] ?? current.status;
    const problem = action === "fail" ? options.problem ?? "sftp_failed" : action === "retry" ? "" : current.problem;
    const updated: TransferJob = {
      ...current, status,
      reconnectAttempt: action === "reconnect" ? current.reconnectAttempt + 1 : current.reconnectAttempt,
      reconnectAt: action === "reconnect" ? new Date(Date.now() + testReconnectDelayMs).toISOString() : "",
      allowedActions: allowedActions(status, problem),
      attempt: action === "retry" ? current.attempt + 1 : current.attempt,
      problem,
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
    recoverySettings,
    listTransfers: vi.fn(async () => ({ maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, ...recoverySettings, jobs: [...jobs.values()] })),
    updateTransferSettings: vi.fn(async (settings: TransferSettings) => ({
      ...settings, jobs: [...jobs.values()],
    })),
    moveTransfer: vi.fn(async () => ({ maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false, largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, ...recoverySettings, jobs: [...jobs.values()] })),
    createTransfer,
    updateTransfer,
    clearFinishedTransfers: vi.fn(async () => {
      for (const [id, job] of jobs) if (job.status === "completed" || job.status === "cancelled") jobs.delete(id);
    }),
    removeTransfer: vi.fn(async (id: string) => { jobs.delete(id); }),
    checkpointDownload: vi.fn(async (id: string, offset: number, revision: string) => {
      const current = jobs.get(id);
      if (current === undefined) throw new Error("sftp_transfer_not_found");
      const updated = { ...current, transferredBytes: offset, downloadRevision: revision };
      jobs.set(id, updated);
      return updated;
    }),
    verifyDownload: vi.fn(async () => undefined),
    startUpload: vi.fn(async ({ id, remotePath: path, size }: UploadStart) => ({
      id, path, offset: 0, size, expectedRevision: "absent", completedRanges: [] as ResumableUpload["completedRanges"], parallelism: 1, chunkBytes: 32 << 20,
    })),
    appendUpload: vi.fn(async ({ id, remotePath: path, offset, total, chunk, range }: UploadChunk): Promise<ResumableUpload> => (range
      ? {
        id, path, offset: offset + chunk.size, size: total, expectedRevision: "absent",
        completedRanges: [{ offset, size: chunk.size }], parallelism: 4, chunkBytes: 8 << 20,
      }
      : { id, path, offset: total, size: total, expectedRevision: "", completedRanges: [], parallelism: 1, chunkBytes: 32 << 20 })),
    completeUpload: vi.fn(async ({ id, size }: UploadCompletion) => {
      const current = jobs.get(id);
      if (current !== undefined) jobs.set(id, { ...current, status: "completed", allowedActions: [], transferredBytes: size, remainingSeconds: 0 });
    }),
    cancelUpload: vi.fn(async ({ id }: UploadTarget) => {
      const current = jobs.get(id);
      if (current !== undefined) jobs.set(id, { ...current, status: "cancelled", allowedActions: [], problem: "" });
    }),
    streamDownload: vi.fn(async (_download: DownloadStream, options: StreamDownloadOptions) => {
      options.onRevision?.('"revision"');
      await options.onChunk(new TextEncoder().encode("data"), 4);
      return { bytes: 4, total: 4 };
    }),
    // Resolves as Android does: every byte is in the saved file on return.
    saveDownload: vi.fn(async (_remotePath: string, _directory: boolean, _parts: BlobPart[]): Promise<BrowserSave | null> => null),
    ...overrides,
  };
}

// The origin private file system, held in memory, as the download part files
// see it.
function privateFileSystem() {
  const files = new Map<string, Uint8Array<ArrayBuffer>>();
  const root = {
    async *values() {
      for (const name of [...files.keys()]) yield { name };
    },
    getFileHandle: vi.fn(async (name: string) => {
      if (!files.has(name)) files.set(name, new Uint8Array());
      return {
        getFile: async () => new File([files.get(name) ?? new Uint8Array()], name),
        createWritable: async () => {
          let contents = files.get(name) ?? new Uint8Array();
          let position = 0;
          return {
            truncate: async (size: number) => { contents = contents.slice(0, size); },
            seek: async (offset: number) => { position = offset; },
            write: async (chunk: Uint8Array) => {
              const grown = new Uint8Array(Math.max(contents.length, position + chunk.length));
              grown.set(contents);
              grown.set(chunk, position);
              contents = grown;
              position += chunk.length;
            },
            close: async () => { files.set(name, contents); },
          };
        },
      };
    }),
    removeEntry: vi.fn(async (name: string) => { files.delete(name); }),
  };
  Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: async () => root } });
  return files;
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

  it("does not let a listing requested before a start put the running job back to queued", async () => {
    const api = engineAPI();
    await api.createTransfer({
      id: "transfer_stale001", batchId: "batch_stale00001", batchName: "remote.bin", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "remote.bin", remotePath: "/remote.bin",
      totalBytes: 4, lastModified: 0,
    });
    // The engine holds new transfers, so the manager adopts the queue without
    // starting anything by itself; the start below is the only one.
    const held = { ...(await api.listTransfers()), processingStopped: true };
    api.listTransfers.mockResolvedValue(held);
    const manager = new SFTPTransferManager(api, 0);
    await manager.reconcile();
    expect(manager.getSnapshot()[0]?.status).toBe("queued");
    // A poll captured the listing while the job was still queued, then a start
    // answered before that listing arrived. The listing is the older fact and
    // must not win, or the browser would re-start an already running job.
    const started = await api.updateTransfer("transfer_stale001", "start");
    expect(started.status).toBe("running");
    const staleListing = manager.reconcile();
    manager["ledger"].replaceServer(started);
    await staleListing;
    expect(manager.getSnapshot()[0]?.status).toBe("running");
  });

  it("counts a waiting upload as this page's transfer while the page holds its File", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api, 0);
    await manager.addDownload("edge", "/queued.bin", "file", 4);
    expect(manager.hasBrowserTransfers()).toBe(false);
    await manager.addUploads([{ alias: "edge", remotePath: "/a.txt", localName: "a.txt", file: new File(["a"], "a.txt") }]);
    expect(manager.hasBrowserTransfers()).toBe(true);
    const upload = manager.getSnapshot().find((job) => job.direction === "upload")!;
    await manager.pause(upload.id);
    expect(manager.hasBrowserTransfers()).toBe(false);
  });

  it("counts a download as this page's transfer only while the page runs it", async () => {
    let finishStream = () => {};
    const api = engineAPI({
      streamDownload: vi.fn(async (_download: DownloadStream, options: StreamDownloadOptions) => {
        options.onRevision?.('"revision"');
        await new Promise<void>((resolve) => { finishStream = resolve; });
        await options.onChunk(new TextEncoder().encode("data"), 4);
        return { bytes: 4, total: 4 };
      }),
    });
    const manager = new SFTPTransferManager(api);
    await manager.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(api.streamDownload).toHaveBeenCalled());
    expect(manager.hasBrowserTransfers()).toBe(true);
    finishStream();
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(manager.hasBrowserTransfers()).toBe(false);
  });

  it("keeps a download added while an older listing was on its way", async () => {
    const api = engineAPI();
    const emptyListing = { ...(await api.listTransfers()), processingStopped: true };
    let answerListing = (_listing: typeof emptyListing) => {};
    api.listTransfers.mockImplementationOnce(() => new Promise((resolve) => { answerListing = resolve; }));
    const manager = new SFTPTransferManager(api, 0);
    const staleListing = manager.reconcile();
    const id = await manager.addDownload("edge", "/remote.bin", "file", 4);
    answerListing(emptyListing);
    await staleListing;
    expect(manager.getSnapshot().map((job) => job.id)).toEqual([id]);
  });

  it("does not bring back removed jobs from a listing requested before the removal", async () => {
    const api = engineAPI();
    for (const id of ["transfer_failed01", "transfer_done0001"]) {
      await api.createTransfer({
        id, batchId: `batch_${id}`, batchName: "remote.bin", batchKind: "file",
        alias: "edge", direction: "download", kind: "file", name: "remote.bin", remotePath: `/${id}`,
        totalBytes: 4, lastModified: 0,
      });
    }
    await api.updateTransfer("transfer_failed01", "fail");
    await api.updateTransfer("transfer_done0001", "complete");
    const manager = new SFTPTransferManager(api, 0);
    await manager.reconcile();
    const olderListing = await api.listTransfers();
    let answerListing = (_listing: typeof olderListing) => {};
    api.listTransfers.mockImplementationOnce(() => new Promise((resolve) => { answerListing = resolve; }));
    const staleListing = manager.reconcile();
    await manager.remove("transfer_failed01");
    await manager.clearFinished();
    answerListing(olderListing);
    await staleListing;
    expect(manager.getSnapshot()).toEqual([]);
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

  it("uploads one large file as independent ranges and resumes completed ranges", async () => {
    const api = engineAPI();
    api.startUpload.mockImplementation(async ({ id, remotePath: path, size }) => ({
      id, path, offset: 2, size, expectedRevision: "absent",
      completedRanges: [{ offset: 0, size: 2 }], parallelism: 2, chunkBytes: 2,
    }));
    const acknowledged: Array<{ offset: number; body: string }> = [];
    api.appendUpload.mockImplementation(async ({ id, remotePath: path, offset, total, chunk }) => {
      acknowledged.push({ offset, body: await chunk.text() });
      return {
        id, path, offset: 2 + acknowledged.reduce((sum, range) => sum + range.body.length, 0), size: total,
        expectedRevision: "absent", completedRanges: [{ offset: 0, size: 2 }, { offset, size: chunk.size }],
        parallelism: 2, chunkBytes: 2,
      };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addUploads([{
      alias: "edge", remotePath: "/large.bin", localName: "large.bin", file: new File(["abcdefgh"], "large.bin"),
    }]);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.appendUpload.mock.calls.every(([chunk]) => chunk.range)).toBe(true);
    expect(acknowledged.sort((left, right) => left.offset - right.offset)).toEqual([
      { offset: 2, body: "cd" }, { offset: 4, body: "ef" }, { offset: 6, body: "gh" },
    ]);
    expect(api.completeUpload).toHaveBeenCalledWith({
      alias: "edge", id: expect.any(String), remotePath: "/large.bin", size: 8, expectedRevision: "absent",
      sourceFingerprint: expect.stringMatching(/^tree-sha256:/),
      signal: expect.any(AbortSignal),
    });
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

  it("keeps a finished download's part file for the browser's save until the job is cleared", async () => {
    const files = privateFileSystem();
    const release = vi.fn();
    const api = engineAPI({ saveDownload: vi.fn(async (): Promise<BrowserSave | null> => ({ release })) });
    const manager = new SFTPTransferManager(api);
    const id = await manager.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    await manager.reconcile();
    expect(files.has(`sshc-sftp-${id}.part`)).toBe(true);
    expect(release).not.toHaveBeenCalled();
    await manager.clearFinished();
    expect(release).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(files.has(`sshc-sftp-${id}.part`)).toBe(false));
  });

  it("lets go of a finished download's save after the retention even while the job stays listed", async () => {
    const files = privateFileSystem();
    const release = vi.fn();
    let clock = 1_000;
    const api = engineAPI({ saveDownload: vi.fn(async (): Promise<BrowserSave | null> => ({ release })) });
    const manager = new SFTPTransferManager(api, 2, () => clock);
    const id = await manager.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    clock += browserSaveRetentionMs - 1;
    await manager.reconcile();
    expect(release).not.toHaveBeenCalled();
    expect(files.has(`sshc-sftp-${id}.part`)).toBe(true);
    clock += 1;
    await manager.reconcile();
    expect(release).toHaveBeenCalledOnce();
    expect(files.has(`sshc-sftp-${id}.part`)).toBe(false);
    expect(manager.getSnapshot()[0]?.status).toBe("completed");
  });

  it("deletes a finished download's part file once the save has written every byte", async () => {
    const files = privateFileSystem();
    const api = engineAPI();
    const manager = new SFTPTransferManager(api);
    const id = await manager.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(files.has(`sshc-sftp-${id}.part`)).toBe(false);
  });

  it("deletes the part file a closed page left for the browser's save", async () => {
    const files = privateFileSystem();
    const api = engineAPI({ saveDownload: vi.fn(async (): Promise<BrowserSave | null> => ({ release: vi.fn() })) });
    const closed = new SFTPTransferManager(api);
    const id = await closed.addDownload("edge", "/remote.bin", "file", 4);
    await vi.waitFor(() => expect(closed.getSnapshot()[0]?.status).toBe("completed"));
    const reopened = new SFTPTransferManager(api);
    await reopened.reconcile();
    expect(files.has(`sshc-sftp-${id}.part`)).toBe(false);
  });

  it("checkpoints a download once its chunks add up, not after every chunk", async () => {
    const api = engineAPI();
    api.streamDownload.mockImplementation(async (_download, options) => {
      options.onRevision?.('"revision-batched"');
      for (const part of ["ab", "cd", "ef"]) await options.onChunk(new TextEncoder().encode(part), 6);
      return { bytes: 6, total: 6 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addDownload("edge", "/batched.bin", "file", 6);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    // Three small chunks fit in one batch: the engine hears about them once, at the end.
    const offsets = api.checkpointDownload.mock.calls.map((call) => call[1]);
    expect(offsets).toEqual([6]);
    const parts = api.saveDownload.mock.calls[0]![2];
    await expect(new Blob(parts).text()).resolves.toBe("abcdef");
  });

  it("resumes a file download after a transient disconnect", async () => {
    let calls = 0;
    const api = engineAPI();
    api.recoverySettings.autoReconnect = true;
    api.recoverySettings.maxReconnectAttempts = 3;
    api.streamDownload.mockImplementation(async ({ offset }, options) => {
      options.onRevision?.('"revision-resume"');
      calls += 1;
      if (calls === 1) {
        expect(offset).toBe(0);
        await options.onChunk(new TextEncoder().encode("abc"), 6);
        throw new Error("sftp_connection_lost");
      }
      expect(offset).toBe(3);
      await options.onChunk(new TextEncoder().encode("def"), 6);
      return { bytes: 6, total: 6 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.addDownload("edge", "/resume.bin", "file", 6);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.streamDownload).toHaveBeenCalledTimes(2);
    const parts = api.saveDownload.mock.calls[0]![2];
    await expect(new Blob(parts).text()).resolves.toBe("abcdef");
  });

  it("stops browser upload recovery at the engine attempt budget", async () => {
    const api = engineAPI();
    api.recoverySettings.autoReconnect = true;
    api.recoverySettings.maxReconnectAttempts = 2;
    api.appendUpload.mockRejectedValue(new Error("sftp_connection_lost"));
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.addUploads([{ alias: "edge", remotePath: "/bounded.bin", localName: "bounded.bin", file: new File(["payload"], "bounded.bin") }]);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("failed"));
    expect(api.appendUpload).toHaveBeenCalledTimes(3);
    expect(manager.getSnapshot()[0]?.reconnectAttempt).toBe(2);
  });

  it("does not replay a browser save whose completion is uncertain", async () => {
    const api = engineAPI();
    api.recoverySettings.autoReconnect = true;
    api.recoverySettings.maxReconnectAttempts = 2;
    api.saveDownload.mockRejectedValue(new TypeError("save acknowledgement lost"));
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.addDownload("edge", "/save-once.bin", "file", 4);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("failed"));
    expect(api.saveDownload).toHaveBeenCalledOnce();
    expect(manager.getSnapshot()[0]?.allowedActions).toEqual(["cancel"]);
    expect(api.streamDownload).toHaveBeenCalledOnce();
    expect(api.updateTransfer.mock.calls.some(([, action]) => action === "reconnect")).toBe(false);
  });

  it("does not replay an upload publication whose acknowledgement is lost", async () => {
    const api = engineAPI();
    api.recoverySettings.autoReconnect = true;
    api.recoverySettings.maxReconnectAttempts = 2;
    api.completeUpload.mockRejectedValue(new TypeError("publication acknowledgement lost"));
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.addUploads([{ alias: "edge", remotePath: "/publish-once.bin", localName: "publish-once.bin", file: new File(["payload"], "publish-once.bin") }]);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("failed"));
    expect(api.completeUpload).toHaveBeenCalledOnce();
    expect(manager.getSnapshot()[0]?.allowedActions).toEqual(["cancel"]);
    expect(api.updateTransfer.mock.calls.some(([, action]) => action === "reconnect")).toBe(false);
  });

  it("checkpoints a download by volume and at the end instead of after every chunk", async () => {
    const api = engineAPI();
    const chunkCount = 64;
    api.streamDownload.mockImplementation(async (_download, options) => {
      options.onRevision?.('"revision-many"');
      for (let index = 0; index < chunkCount; index += 1) {
        await options.onChunk(new Uint8Array(1024), chunkCount * 1024);
      }
      return { bytes: chunkCount * 1024, total: chunkCount * 1024 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addDownload("edge", "/many.bin", "file", chunkCount * 1024);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    // Sixty-four small chunks arrive within the checkpoint interval, so the
    // engine hears about the position once at the end (plus the final
    // acknowledgement before saving), not sixty-four times.
    expect(api.checkpointDownload.mock.calls.length).toBeLessThanOrEqual(2);
    const parts = api.saveDownload.mock.calls[0]![2];
    expect(new Blob(parts).size).toBe(chunkCount * 1024);
  });

  it("retries only failed uploads in an engine-owned folder batch", async () => {
    let failBad = true;
    const api = engineAPI();
    api.startUpload.mockImplementation(async ({ id, remotePath: path, size }) => {
      if (path.endsWith("bad.txt") && failBad) throw new Error("sftp_connection_lost");
      return { id, path, offset: 0, size, expectedRevision: "absent", completedRanges: [], parallelism: 1, chunkBytes: 32 << 20 };
    });
    const manager = new SFTPTransferManager(api);
    await manager.addUploads([
      { alias: "edge", remotePath: "/project/good.txt", localName: "project/good.txt", file: new File(["good"], "good.txt") },
      { alias: "edge", remotePath: "/project/bad.txt", localName: "project/bad.txt", file: new File(["bad"], "bad.txt") },
    ], { id: "batch_project1", name: "project", kind: "folder" });
    await vi.waitFor(() => expect(manager.getSnapshot().map((job) => job.status).sort()).toEqual(["completed", "failed"]));
    const goodCalls = () => api.startUpload.mock.calls.filter((call) => call[0].remotePath.endsWith("good.txt")).length;
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
    api.appendUpload.mockImplementation(async ({ id, remotePath: path, total }) => {
      await new Promise<void>((resolve) => releases.push(resolve));
      return { id, path, offset: total, size: total, expectedRevision: "", completedRanges: [], parallelism: 1, chunkBytes: 32 << 20 };
    });
    api.streamDownload.mockImplementation(async (_download, options) => {
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

  it("lets an engine-side copy that met an existing target be overwritten", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api, 0);
    const [id] = await manager.addRemoteTransfers([{
      sourceAlias: "edge", sourcePath: "/srv/report.txt", targetAlias: "nas", targetPath: "/backup/report.txt",
      name: "report.txt", kind: "file", totalBytes: 12,
    }], "copy");
    // The engine stopped the copy because the target exists.
    const stopped = await api.updateTransfer(id!, "needs_overwrite");
    expect(stopped.status).toBe("needs_overwrite");
    await manager.reconcile();

    await manager.overwrite(id!);

    expect(api.updateTransfer).toHaveBeenLastCalledWith(id, "resume", { resetProgress: false });
    expect(api.jobs.get(id!)?.overwrite).toBe(true);
    expect(manager.getSnapshot()[0]?.status).toBe("queued");
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

  it("removes one failed job through the engine without clearing other history", async () => {
    const api = engineAPI();
    const first = await api.createTransfer({
      id: "transfer_failed1", batchId: "batch_failed01", batchName: "failed", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "failed", remotePath: "/failed", totalBytes: 4, lastModified: 0,
    });
    const second = await api.createTransfer({
      id: "transfer_done01", batchId: "batch_done0001", batchName: "done", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "done", remotePath: "/done", totalBytes: 4, lastModified: 0,
    });
    api.jobs.set(first.id, { ...first, status: "failed", allowedActions: ["retry", "cancel", "remove"], problem: "network" });
    api.jobs.set(second.id, { ...second, status: "completed", allowedActions: ["remove"] });
    const manager = new SFTPTransferManager(api, 0);
    await manager.reconcile();
    await manager.remove(first.id);
    expect(manager.getSnapshot().map((job) => job.id)).toEqual([second.id]);
    expect(api.removeTransfer).toHaveBeenCalledWith(first.id);
  });

  it("applies pause, resume, and cancel to every actionable transfer", async () => {
    const api = engineAPI();
    const manager = new SFTPTransferManager(api, 0);
    await manager.addDownload("edge", "/first.bin", "file", 4);
    await manager.addDownload("edge", "/second.bin", "file", 4);

    await manager.pauseAll();
    expect(manager.getSnapshot().map((job) => job.status)).toEqual(["paused", "paused"]);
    await manager.resumeAll();
    expect(manager.getSnapshot().map((job) => job.status)).toEqual(["queued", "queued"]);
    await manager.cancelAll();
    expect(manager.getSnapshot().map((job) => job.status)).toEqual(["cancelled", "cancelled"]);
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
    expect(api.verifyDownload).toHaveBeenCalledWith({ alias: "edge", jobId: created.id, remotePath: "/full.bin" }, '"content-sha256:full"');
    expect(api.streamDownload).not.toHaveBeenCalled();
    expect(api.saveDownload).toHaveBeenCalledOnce();
  });

  it("cuts an OPFS part file back to the engine offset when the engine refuses the longer checkpoint", async () => {
    const body = new Uint8Array([1, 2, 3, 4, 5, 6]);
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
    const api = engineAPI({
      checkpointDownload: vi.fn(async () => { throw new Error("sftp_range_invalid"); }),
    });
    const created = await api.createTransfer({
      id: "transfer_longopfs", batchId: "batch_longopfs", batchName: "long.bin", batchKind: "file",
      alias: "edge", direction: "download", kind: "file", name: "long.bin", remotePath: "/long.bin",
      totalBytes: 8, lastModified: 0,
    });
    api.jobs.set(created.id, {
      ...created, transferredBytes: 4, downloadRevision: '"content-sha256:long"',
      status: "paused", allowedActions: ["resume", "cancel"],
    });
    const manager = new SFTPTransferManager(api);
    await manager.reconcile();
    await manager.resume(created.id);
    await vi.waitFor(() => expect(api.streamDownload).toHaveBeenCalled());
    expect(writer.truncate).toHaveBeenCalledWith(4);
    expect(writer.truncate).not.toHaveBeenCalledWith(0);
    expect(api.streamDownload.mock.calls[0]?.[0].offset).toBe(4);
    expect(api.updateTransfer).not.toHaveBeenCalledWith(created.id, "progress", expect.objectContaining({ resetProgress: true }));
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

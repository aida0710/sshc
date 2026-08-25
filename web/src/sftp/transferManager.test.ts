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
  checkpointDownload: vi.fn(async (id: string, offset: number): Promise<TransferJob> => {
    const current = jobs.get(id);
    if (current === undefined) throw new Error("not_found");
    const updated = { ...current, transferredBytes: offset, totalBytes: current.totalBytes < 0 ? offset : current.totalBytes };
    jobs.set(id, updated);
    return updated;
  }),
  verifyDownload: vi.fn(async () => undefined),
    startUpload: vi.fn(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "absent" })),
    appendUpload: vi.fn(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({ id, path, offset: total, size: total, expectedRevision: "" })),
    completeUpload: vi.fn(async (_alias: string, _id: string, _path: string, _size: number, _expectedRevision: string, _sourceFingerprint: string) => undefined),
    cancelUpload: vi.fn(async () => undefined),
    streamDownload: vi.fn(async (_alias: string, _id: string, _path: string, _directory: boolean, _offset: number, _options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }> => ({ bytes: 0, total: 0 })),
    saveDownload: vi.fn((_remotePath: string, _directory: boolean, _parts: BlobPart[]) => undefined),
    ...overrides,
  };
}

describe("SFTPTransferManager", () => {
  beforeEach(() => {
    localStorage.clear();
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: undefined });
  });

  it("does not create persistent storage for an empty queue", async () => {
    const manager = new SFTPTransferManager(serverAPI());
    await manager.reconcile();
    expect(localStorage.getItem("sshc.sftp.transfer-manager.v3")).toBeNull();
  });

  it("does not read the removed v1 or v2 queue formats", () => {
    const obsolete = JSON.stringify([{ id: "obsolete", status: "queued" }]);
    localStorage.setItem("sshc.sftp.transfer-queue.v1", obsolete);
    localStorage.setItem("sshc.sftp.transfer-manager.v2", obsolete);
    const manager = new SFTPTransferManager(serverAPI());
    expect(manager.getSnapshot()).toHaveLength(0);
  });

  it("resumes a failed file download with Range state and reports speed and completion", async () => {
    let now = 0;
    let calls = 0;
    const api = serverAPI();
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, offset, options) => {
    options.onRevision?.('"revision-1"');
      calls += 1;
      if (calls === 1) {
        expect(offset).toBe(0);
        now = 1_000;
        await options.onChunk(new Uint8Array(1 << 20), 2 << 20);
        throw new Error("connection_lost");
      }
      expect(offset).toBe(1 << 20);
      now = 2_000;
      await options.onChunk(new Uint8Array(1 << 20), 2 << 20);
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

  it("restarts a failed folder archive from byte zero without retaining the partial ZIP", async () => {
    const api = serverAPI();
    let attempt = 0;
    api.streamDownload.mockImplementation(async (_alias, _id, _path, directory, offset, options) => {
      options.onRevision?.(`"archive-${attempt + 1}"`);
      expect(directory).toBe(true);
      expect(offset).toBe(0);
      attempt += 1;
      if (attempt === 1) {
        await options.onChunk(new TextEncoder().encode("partial"), null);
        throw new Error("connection_lost");
      }
      await options.onChunk(new TextEncoder().encode("complete-zip"), null);
      return { bytes: 12, total: null };
    });
    const manager = new SFTPTransferManager(api);
    manager.addDownload("edge", "/remote/folder", "folder", -1);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("failed"));
    manager.retry(manager.getSnapshot()[0]!.id);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    const parts = api.saveDownload.mock.calls[0]![2];
    await expect(new Blob(parts).text()).resolves.toBe("complete-zip");
  });

  it("applies one concurrency limit to uploads and downloads", async () => {
    const uploadReleases: Array<() => void> = [];
    let releaseDownload: (() => void) | undefined;
    const api = serverAPI();
    api.appendUpload.mockImplementation(async (_alias, id, path, _offset, total) => {
      await new Promise<void>((resolve) => uploadReleases.push(resolve));
      return { id, path, offset: total, size: total, expectedRevision: "" };
    });
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, offset, options) => {
    options.onRevision?.('"revision-limit"');
      await new Promise<void>((resolve) => { releaseDownload = resolve; });
      await options.onChunk(new Uint8Array(4), 4);
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

  it("removes orphan OPFS parts while preserving an active persisted job", async () => {
    const sizes = new Map<string, number>([["sshc-sftp-orphan_job.part", 4]]);
    const removed: string[] = [];
    const root = {
    async *values() { for (const name of sizes.keys()) yield { name, kind: "file" }; },
    removeEntry: vi.fn(async (name: string) => { removed.push(name); sizes.delete(name); }),
    getFileHandle: vi.fn(),
    };
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: vi.fn(async () => root) } });
    const manager = new SFTPTransferManager(serverAPI());
    const activeID = manager.addDownload("edge", "/active.bin", "file", 4);
    manager.pause(activeID);
    sizes.set(`sshc-sftp-${activeID}.part`, 4);
    await manager.reconcile();
    expect(removed).toContain("sshc-sftp-orphan_job.part");
    expect(removed).not.toContain(`sshc-sftp-${activeID}.part`);
    await manager.cancel(activeID);
    expect(removed).toContain(`sshc-sftp-${activeID}.part`);
  });

  it("finishes a fully durable OPFS file after revision verification without an EOF Range", async () => {
    const body = new Uint8Array([1, 2, 3, 4]);
    let size = body.byteLength;
    const writer = {
    truncate: vi.fn(async (next: number) => { size = next; }),
    seek: vi.fn(async () => undefined), write: vi.fn(async () => undefined), close: vi.fn(async () => undefined),
    };
    const handle = {
    createWritable: vi.fn(async () => writer),
    getFile: vi.fn(async () => new File([body.slice(0, size)], "part")),
    };
    const root = {
    async *values() { yield { name: "sshc-sftp-unrelated.part", kind: "file" }; },
    getFileHandle: vi.fn(async () => handle), removeEntry: vi.fn(async () => undefined),
    };
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: vi.fn(async () => root) } });
    const now = new Date().toISOString();
    localStorage.setItem("sshc.sftp.transfer-manager.v3", JSON.stringify([{
    id: "transfer_fullopfs", batchId: "batch_fullopfs", alias: "edge", direction: "download", kind: "file",
    name: "full.bin", remotePath: "/full.bin", totalBytes: 4, transferredBytes: 4, bytesPerSecond: 0,
    remainingSeconds: 0, status: "failed", attempt: 1, problem: "connection_lost", createdAt: now, updatedAt: now,
    batchName: "full.bin", batchKind: "file", lastModified: 0, expectedRevision: "", overwrite: false,
    downloadRevision: '"content-sha256:full"', sourceFingerprint: "",
    }]));
    const api = serverAPI();
    const manager = new SFTPTransferManager(api);
    manager.retry("transfer_fullopfs");
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.verifyDownload).toHaveBeenCalledWith("edge", "transfer_fullopfs", "/full.bin", '"content-sha256:full"');
    expect(api.streamDownload).not.toHaveBeenCalled();
    expect(api.saveDownload).toHaveBeenCalledOnce();
  });

  it("restarts a complete OPFS file from zero when the engine has no sent-byte evidence", async () => {
    let size = 4;
    const writer = {
      truncate: vi.fn(async (next: number) => { size = next; }),
      seek: vi.fn(async () => undefined),
      write: vi.fn(async (value: Uint8Array) => { size += value.byteLength; }),
      close: vi.fn(async () => undefined),
    };
    const handle = {
      createWritable: vi.fn(async () => writer),
      getFile: vi.fn(async () => new File([new Uint8Array(size)], "part")),
    };
    const root = {
      async *values() { /* no orphan entries */ },
      getFileHandle: vi.fn(async () => handle), removeEntry: vi.fn(async () => undefined),
    };
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: vi.fn(async () => root) } });
    const now = new Date().toISOString();
    localStorage.setItem("sshc.sftp.transfer-manager.v3", JSON.stringify([{
      id: "transfer_restart", batchId: "batch_restart1", alias: "edge", direction: "download", kind: "file",
      name: "full.bin", remotePath: "/full.bin", totalBytes: 4, transferredBytes: 4, bytesPerSecond: 0,
      remainingSeconds: 0, status: "failed", attempt: 1, problem: "connection_lost", createdAt: now, updatedAt: now,
      batchName: "full.bin", batchKind: "file", lastModified: 0, expectedRevision: "", overwrite: false,
      downloadRevision: '"content-sha256:old"', sourceFingerprint: "",
    }]));
    const api = serverAPI();
    api.verifyDownload.mockRejectedValue(new Error("no_sent_evidence"));
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, offset, options) => {
      expect(offset).toBe(0);
      options.onRevision?.('"content-sha256:new"');
      await options.onChunk(new Uint8Array(4), 4);
      return { bytes: 4, total: 4 };
    });
    const manager = new SFTPTransferManager(api);
    manager.retry("transfer_restart");
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(writer.truncate).toHaveBeenCalledWith(0);
    expect(api.streamDownload).toHaveBeenCalledOnce();
  });

  it("computes the upload fingerprint before registering a server job", async () => {
    const api = serverAPI();
    const originalDigest = globalThis.crypto.subtle.digest.bind(globalThis.crypto.subtle);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    const digest = vi.spyOn(globalThis.crypto.subtle, "digest").mockImplementation(async (...args) => {
    await gate;
    return originalDigest(...args);
    });
    const manager = new SFTPTransferManager(api);
    manager.addUploads([{ alias: "edge", remotePath: "/slow.bin", localName: "slow.bin", file: new File(["slow"], "slow.bin") }]);
    await Promise.resolve();
    expect(api.createTransfer).not.toHaveBeenCalled();
    release();
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.createTransfer).toHaveBeenCalledOnce();
    digest.mockRestore();
  });

  it("stops fingerprinting promptly when an upload is paused", async () => {
    const api = serverAPI();
    const originalDigest = globalThis.crypto.subtle.digest.bind(globalThis.crypto.subtle);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    const digest = vi.spyOn(globalThis.crypto.subtle, "digest").mockImplementation(async (...args) => {
      await gate;
      return originalDigest(...args);
    });
    const manager = new SFTPTransferManager(api);
    manager.addUploads([{ alias: "edge", remotePath: "/pause.bin", localName: "pause.bin", file: new File(["pause"], "pause.bin") }]);
    await vi.waitFor(() => expect(digest).toHaveBeenCalled());
    const id = manager.getSnapshot()[0]!.id;
    manager.pause(id);
    release();
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("paused"));
    expect(api.createTransfer).not.toHaveBeenCalled();
    digest.mockRestore();
  });

  it("keeps the durable in-memory prefix when a fallback download is paused and resumed", async () => {
    const api = serverAPI();
    let attempt = 0;
    api.streamDownload.mockImplementation(async (_alias, _id, _path, _directory, offset, options) => {
      attempt += 1;
      options.onRevision?.('"fallback-revision"');
      if (attempt === 1) {
        expect(offset).toBe(0);
        await options.onChunk(new TextEncoder().encode("abc"), 6);
        if (options.signal?.aborted) throw new DOMException("paused", "AbortError");
        await new Promise<void>((_resolve, reject) => {
          options.signal?.addEventListener("abort", () => reject(new DOMException("paused", "AbortError")), { once: true });
        });
      }
      expect(offset).toBe(3);
      await options.onChunk(new TextEncoder().encode("def"), 6);
      return { bytes: 3, total: 6 };
    });
    const manager = new SFTPTransferManager(api);
    const id = manager.addDownload("edge", "/fallback.bin", "file", 6);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.transferredBytes).toBe(3));
    manager.pause(id);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("paused"));
    manager.resume(id);
    await vi.waitFor(() => expect(["completed", "failed"]).toContain(manager.getSnapshot()[0]?.status));
    expect(manager.getSnapshot()[0]).toMatchObject({ status: "completed", problem: "" });
    const parts = api.saveDownload.mock.calls[0]![2];
    await expect(new Blob(parts).text()).resolves.toBe("abcdef");
  });

  it("rejects a 201st browser queue admission without silently dropping existing jobs", () => {
    const manager = new SFTPTransferManager(serverAPI(), 0);
    for (let index = 0; index < 200; index += 1) {
      manager.addDownload("edge", `/file-${index}.bin`, "file", 1);
    }
    expect(manager.getSnapshot()).toHaveLength(200);
    expect(() => manager.addDownload("edge", "/overflow.bin", "file", 1)).toThrow("sftp_transfer_limit");
    expect(manager.getSnapshot()).toHaveLength(200);
  });

  it("rejects an oversized persisted queue instead of restoring more than the hard limit", () => {
    const now = new Date().toISOString();
    const jobs = Array.from({ length: 201 }, (_, index) => ({
      id: `transfer_restore${index}`, batchId: "batch_restore01", alias: "edge", direction: "download", kind: "file",
      name: `file-${index}.bin`, remotePath: `/file-${index}.bin`, totalBytes: 1, transferredBytes: 0,
      bytesPerSecond: 0, remainingSeconds: -1, status: "failed", attempt: 1, problem: "sftp_transfer_limit",
      createdAt: now, updatedAt: now, batchName: "restore", batchKind: "file", lastModified: 0,
      expectedRevision: "", downloadRevision: "", sourceFingerprint: "", overwrite: false,
    }));
    localStorage.setItem("sshc.sftp.transfer-manager.v3", JSON.stringify(jobs));
    const manager = new SFTPTransferManager(serverAPI(), 0);
    expect(manager.getSnapshot()).toHaveLength(0);
    expect(JSON.parse(localStorage.getItem("sshc.sftp.transfer-manager.v3") ?? "[]")).toHaveLength(201);
  });

  it("keeps the browser queue within the hard limit while reconciling server jobs", async () => {
    const manager = new SFTPTransferManager(serverAPI(), 0);
    for (let index = 0; index < 200; index += 1) {
      manager.addDownload("edge", `/local-${index}.bin`, "file", 1);
    }
    const api = serverAPI();
    const now = new Date().toISOString();
    api.listTransfers.mockResolvedValue({
      maxConcurrent: 2,
      jobs: [{
        id: "transfer_server_only", batchId: "batch_server_only", alias: "edge", direction: "download", kind: "file",
        name: "server.bin", remotePath: "/server.bin", totalBytes: 1, transferredBytes: 0,
        bytesPerSecond: 0, remainingSeconds: -1, status: "queued", attempt: 1, problem: "",
        createdAt: now, updatedAt: now,
      }],
    });
    const restored = new SFTPTransferManager(api, 0);
    // Reuse the exact full persisted queue created above, then merge one
    // authoritative server-only job without ever exposing 201 entries.
    await restored.reconcile();
    expect(restored.getSnapshot()).toHaveLength(200);
    expect(restored.getSnapshot().some((job) => job.id === "transfer_server_only")).toBe(true);
  });

  it("converges to completed when the upload complete response is lost", async () => {
    const api = serverAPI();
    const updateImplementation = api.updateTransfer.getMockImplementation()!;
    api.updateTransfer.mockImplementation(async (id, action, options) => {
      const current = (await api.listTransfers()).jobs.find((job) => job.id === id);
      if (current?.status === "completed" && action === "fail") throw new Error("sftp_transfer_state");
      return updateImplementation(id, action, options);
    });
    api.completeUpload.mockImplementation(async (_alias, id, _path, size) => {
      await updateImplementation(id, "complete", { transferredBytes: size });
      throw new Error("response_lost");
    });
    const manager = new SFTPTransferManager(api);
    manager.addUploads([{ alias: "edge", remotePath: "/response.bin", localName: "response.bin", file: new File(["done"], "response.bin") }]);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("failed"));
    const id = manager.getSnapshot()[0]!.id;
    manager.retry(id);
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(api.completeUpload).toHaveBeenCalledOnce();
  });

  it("keeps an upload cleanup failure visible and allows cancel retry", async () => {
    const api = serverAPI();
    api.cancelUpload.mockRejectedValueOnce(new Error("cleanup_failed"));
    const manager = new SFTPTransferManager(api, 0);
    manager.addUploads([{ alias: "edge", remotePath: "/cleanup.bin", localName: "cleanup.bin", file: new File(["part"], "cleanup.bin") }]);
    const id = manager.getSnapshot()[0]!.id;
    await manager.cancel(id);
    expect(manager.getSnapshot()[0]).toMatchObject({ status: "failed", problem: "sftp_cleanup_pending" });
    await manager.cancel(id);
    expect(manager.getSnapshot()[0]).toMatchObject({ status: "cancelled", problem: "" });
    expect(api.cancelUpload).toHaveBeenCalledTimes(2);
  });

  it("treats cancellation before server job creation as idempotent for uploads and downloads", async () => {
    const uploadAPI = serverAPI();
    uploadAPI.cancelUpload.mockRejectedValue(new Error("sftp_transfer_not_found"));
    const uploadManager = new SFTPTransferManager(uploadAPI, 0);
    uploadManager.addUploads([{ alias: "edge", remotePath: "/queued.bin", localName: "queued.bin", file: new File(["queued"], "queued.bin") }]);
    await uploadManager.cancel(uploadManager.getSnapshot()[0]!.id);
    expect(uploadManager.getSnapshot()[0]).toMatchObject({ status: "cancelled", problem: "" });

    const downloadAPI = serverAPI();
    downloadAPI.updateTransfer.mockRejectedValue(new Error("sftp_transfer_not_found"));
    const downloadManager = new SFTPTransferManager(downloadAPI, 0);
    const downloadID = downloadManager.addDownload("edge", "/queued-download.bin", "file", 1);
    await downloadManager.cancel(downloadID);
    expect(downloadManager.getSnapshot()[0]).toMatchObject({ status: "cancelled", problem: "" });
  });

  it("cancels an upload while fingerprinting before a server job exists", async () => {
    const api = serverAPI();
    api.cancelUpload.mockRejectedValue(new Error("sftp_transfer_not_found"));
    const originalDigest = globalThis.crypto.subtle.digest.bind(globalThis.crypto.subtle);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    const digest = vi.spyOn(globalThis.crypto.subtle, "digest").mockImplementation(async (...args) => {
      await gate;
      return originalDigest(...args);
    });
    const manager = new SFTPTransferManager(api);
    manager.addUploads([{ alias: "edge", remotePath: "/fingerprint.bin", localName: "fingerprint.bin", file: new File(["fingerprint"], "fingerprint.bin") }]);
    await vi.waitFor(() => expect(digest).toHaveBeenCalled());
    const id = manager.getSnapshot()[0]!.id;
    await manager.cancel(id);
    release();
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("cancelled"));
    expect(api.createTransfer).not.toHaveBeenCalled();
    digest.mockRestore();
  });

  it("reserves upload capacity before asynchronous directory creation can consume the slot", () => {
    const manager = new SFTPTransferManager(serverAPI(), 0);
    for (let index = 0; index < 199; index += 1) manager.addDownload("edge", `/existing-${index}`, "file", 1);
    const selection = [{ alias: "edge", remotePath: "/folder/file", localName: "folder/file", file: new File(["file"], "file") }];
    const admission = manager.reserveUploads(selection);
    expect(() => manager.addDownload("edge", "/steal-slot", "file", 1)).toThrow("sftp_transfer_limit");
    manager.addUploads(selection, { name: "folder", kind: "folder" }, admission);
    expect(manager.getSnapshot()).toHaveLength(200);
  });

  it("reloads a partial folder archive from zero and restarts it automatically", async () => {
    let size = 7;
    const writer = {
      truncate: vi.fn(async (next: number) => { size = next; }),
      seek: vi.fn(async () => undefined),
      write: vi.fn(async (value: Uint8Array) => { size += value.byteLength; }),
      close: vi.fn(async () => undefined),
    };
    const handle = {
      createWritable: vi.fn(async () => writer),
      getFile: vi.fn(async () => new File([new Uint8Array(size)], "folder.part")),
    };
    const root = {
      async *values() { /* active part is retained */ },
      getFileHandle: vi.fn(async () => handle),
      removeEntry: vi.fn(async () => undefined),
    };
    Object.defineProperty(globalThis.navigator, "storage", { configurable: true, value: { getDirectory: vi.fn(async () => root) } });
    const now = new Date().toISOString();
    localStorage.setItem("sshc.sftp.transfer-manager.v3", JSON.stringify([{
      id: "transfer_folderreload", batchId: "batch_folderreload", alias: "edge", direction: "download", kind: "folder",
      name: "folder", remotePath: "/folder", totalBytes: 7, transferredBytes: 7, bytesPerSecond: 0,
      remainingSeconds: 0, status: "failed", attempt: 1, problem: "connection_lost", createdAt: now, updatedAt: now,
      batchName: "folder", batchKind: "folder", lastModified: 0, expectedRevision: "", overwrite: false,
      downloadRevision: '"old-archive"', sourceFingerprint: "",
    }]));
    const api = serverAPI();
    api.streamDownload.mockImplementation(async (_alias, _id, _path, directory, offset, options) => {
      expect(directory).toBe(true);
      expect(offset).toBe(0);
      options.onRevision?.('"new-archive"');
      await options.onChunk(new Uint8Array(4), 4);
      return { bytes: 4, total: 4 };
    });
    const manager = new SFTPTransferManager(api);
    expect(manager.getSnapshot()[0]).toMatchObject({ status: "queued", transferredBytes: 0, downloadRevision: "" });
    await manager.reconcile();
    await vi.waitFor(() => expect(manager.getSnapshot()[0]?.status).toBe("completed"));
    expect(writer.truncate).toHaveBeenCalledWith(0);
    expect(api.updateTransfer).toHaveBeenCalledWith("transfer_folderreload", "progress", { transferredBytes: 0, resetProgress: true });
    expect(api.streamDownload).toHaveBeenCalledOnce();
  });
});

import { failureCode } from "../api/client";
import {
  sftpApi,
  type CreateTransferJob,
  type ResumableUpload,
  type StreamDownloadOptions,
  type TransferJob,
  type TransferJobAction,
  type TransferKind,
} from "./api";

const chunkBytes = 1 << 20;
const persistenceKey = "sshc.sftp.transfer-manager.v2";
const legacyUploadKey = "sshc.sftp.transfer-queue.v1";

export type ManagedTransferJob = TransferJob & {
  batchName: string;
  batchKind: TransferKind;
  lastModified: number;
  expectedRevision: string;
  overwrite: boolean;
};

export type UploadSelection = {
  alias: string;
  remotePath: string;
  localName: string;
  file: File;
};

export type TransferNotice = {
  id: string;
  jobId: string;
  status: "completed" | "failed";
  name: string;
  direction: "upload" | "download";
  problem: string;
};

type ManagerAPI = {
  listTransfers(): Promise<{ maxConcurrent: number; jobs: TransferJob[] }>;
  createTransfer(input: CreateTransferJob): Promise<TransferJob>;
  updateTransfer(id: string, action: TransferJobAction, options?: { transferredBytes?: number; problem?: string; resetProgress?: boolean }): Promise<TransferJob>;
  startUpload(alias: string, id: string, remotePath: string, size: number, overwrite: boolean, expectedRevision?: string): Promise<ResumableUpload>;
  appendUpload(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload>;
  completeUpload(alias: string, id: string, remotePath: string, size: number, expectedRevision: string): Promise<void>;
  cancelUpload(alias: string, id: string, remotePath: string): Promise<void>;
  streamDownload(alias: string, remotePath: string, directory: boolean, offset: number, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }>;
  saveDownload(remotePath: string, directory: boolean, chunks: Uint8Array[]): void;
};

type StoredJob = Omit<ManagedTransferJob, "bytesPerSecond" | "remainingSeconds">;

function identifier(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function isoNow(): string {
  return new Date().toISOString();
}

function baseName(remotePath: string): string {
  return remotePath.split("/").filter(Boolean).at(-1) ?? remotePath;
}

function restorable(value: unknown): ManagedTransferJob[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((candidate) => {
    if (candidate === null || typeof candidate !== "object") return [];
    const job = candidate as Partial<StoredJob>;
    if (typeof job.id !== "string" || typeof job.batchId !== "string" || typeof job.alias !== "string" ||
        (job.direction !== "upload" && job.direction !== "download") || (job.kind !== "file" && job.kind !== "folder") ||
        typeof job.name !== "string" || typeof job.remotePath !== "string" || typeof job.totalBytes !== "number" ||
        typeof job.transferredBytes !== "number" || typeof job.status !== "string" || typeof job.attempt !== "number" ||
        typeof job.batchName !== "string" || (job.batchKind !== "file" && job.batchKind !== "folder") ||
        typeof job.lastModified !== "number" || typeof job.expectedRevision !== "string" || typeof job.overwrite !== "boolean") return [];
    let status = job.status;
    let transferredBytes = job.transferredBytes;
    if (!['completed', 'cancelled', 'failed', 'needs_overwrite'].includes(status)) status = "reattach";
    if (job.direction === "download" && status === "reattach") transferredBytes = 0;
    return [{
      ...job as StoredJob, status, transferredBytes, bytesPerSecond: 0, remainingSeconds: -1,
      problem: typeof job.problem === "string" ? job.problem : "",
      createdAt: typeof job.createdAt === "string" ? job.createdAt : isoNow(),
      updatedAt: typeof job.updatedAt === "string" ? job.updatedAt : isoNow(),
    }];
  }).slice(-200);
}

function migrateLegacy(value: unknown): ManagedTransferJob[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((candidate) => {
    if (candidate === null || typeof candidate !== "object") return [];
    const old = candidate as Record<string, unknown>;
    if (typeof old.id !== "string" || typeof old.alias !== "string" || typeof old.remotePath !== "string" ||
        typeof old.localName !== "string" || typeof old.size !== "number" || typeof old.lastModified !== "number" ||
        typeof old.offset !== "number") return [];
    const terminal = old.status === "done" ? "completed" : old.status === "cancelled" ? "cancelled" :
      old.status === "failed" ? "failed" : old.status === "needs_overwrite" ? "needs_overwrite" : "reattach";
    const now = isoNow();
    return [{
      id: old.id, batchId: old.id, alias: old.alias, direction: "upload", kind: "file",
      name: old.localName, remotePath: old.remotePath, totalBytes: old.size, transferredBytes: old.offset,
      bytesPerSecond: 0, remainingSeconds: -1, status: terminal, attempt: 1,
      problem: typeof old.problem === "string" ? old.problem : "", createdAt: now, updatedAt: now,
      batchName: old.localName, batchKind: "file", lastModified: old.lastModified,
      expectedRevision: typeof old.expectedRevision === "string" ? old.expectedRevision : "",
      overwrite: old.overwrite === true,
    } as ManagedTransferJob];
  });
}

function networkReady(job: ManagedTransferJob, files: Map<string, File>): boolean {
  return job.direction === "download" || files.has(job.id);
}

export class SFTPTransferManager {
  private jobs: ManagedTransferJob[] = [];
  private notices: TransferNotice[] = [];
  private readonly files = new Map<string, File>();
  private readonly downloadChunks = new Map<string, Uint8Array[]>();
  private readonly controllers = new Map<string, AbortController>();
  private readonly samples = new Map<string, { at: number; bytes: number }>();
  private readonly serverSamples = new Map<string, { at: number; bytes: number }>();
  private readonly progressReports = new Map<string, Promise<void>>();
  private readonly controlOperations = new Map<string, Promise<void>>();
  private readonly listeners = new Set<() => void>();
  private readonly noticeListeners = new Set<() => void>();
  private active = 0;

  constructor(
    private readonly api: ManagerAPI = sftpApi,
    private readonly concurrency = 2,
    private readonly now = () => Date.now(),
  ) {
    try {
      const stored = globalThis.localStorage?.getItem(persistenceKey);
      this.jobs = restorable(JSON.parse(stored ?? "[]"));
      if (this.jobs.length === 0) {
        this.jobs = migrateLegacy(JSON.parse(globalThis.localStorage?.getItem(legacyUploadKey) ?? "[]"));
      }
    } catch {
      this.jobs = [];
    }
  }

  getSnapshot = (): readonly ManagedTransferJob[] => this.jobs;
  getNoticeSnapshot = (): readonly TransferNotice[] => this.notices;
  getMaxConcurrent = (): number => this.concurrency;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  subscribeNotices = (listener: () => void): (() => void) => {
    this.noticeListeners.add(listener);
    return () => this.noticeListeners.delete(listener);
  };

  async reconcile(): Promise<void> {
    const listed = await this.api.listTransfers();
    const merged = [...this.jobs];
    for (const serverJob of listed.jobs) {
      const index = merged.findIndex((job) => job.id === serverJob.id);
      const local = index < 0 ? undefined : merged[index];
      let status = serverJob.status;
      let transferredBytes = serverJob.transferredBytes;
      if (local?.status === "running") continue;
      if (serverJob.direction === "upload" && !this.files.has(serverJob.id) && !["completed", "cancelled", "failed", "needs_overwrite"].includes(status)) {
        status = "reattach";
      } else if (serverJob.direction === "download" && status === "running") {
        status = "queued";
        transferredBytes = 0;
      }
      const restored: ManagedTransferJob = {
        ...serverJob, status, transferredBytes,
        batchName: local?.batchName ?? serverJob.name, batchKind: local?.batchKind ?? serverJob.kind,
        lastModified: local?.lastModified ?? 0, expectedRevision: local?.expectedRevision ?? "",
        overwrite: local?.overwrite ?? false,
      };
      if (index < 0) merged.push(restored);
      else merged[index] = restored;
    }
    this.commit(merged.slice(-200));
    this.kick();
  }

  addUploads(selections: UploadSelection[], batch?: { id?: string; name: string; kind: TransferKind }): string {
    const batchId = batch?.id ?? identifier("batch");
    const batchName = batch?.name ?? selections[0]?.localName ?? "upload";
    const batchKind = batch?.kind ?? (selections.length > 1 ? "folder" : "file");
    const additions: ManagedTransferJob[] = [];
    for (const selection of selections) {
      const existing = this.jobs.find((job) => job.direction === "upload" && job.alias === selection.alias &&
        job.remotePath === selection.remotePath && job.totalBytes === selection.file.size &&
        job.lastModified === selection.file.lastModified && job.status === "reattach");
      if (existing !== undefined) {
        this.files.set(existing.id, selection.file);
        this.replace(existing.id, { status: "queued", problem: "" });
        continue;
      }
      const id = identifier("transfer");
      const now = isoNow();
      const job: ManagedTransferJob = {
        id, batchId, alias: selection.alias, direction: "upload", kind: "file", name: selection.localName,
        remotePath: selection.remotePath, totalBytes: selection.file.size, transferredBytes: 0,
        bytesPerSecond: 0, remainingSeconds: -1, status: "queued", attempt: 1, problem: "",
        createdAt: now, updatedAt: now, batchName, batchKind, lastModified: selection.file.lastModified,
        expectedRevision: "", overwrite: false,
      };
      this.files.set(id, selection.file);
      additions.push(job);
    }
    if (additions.length > 0) this.commit([...this.jobs, ...additions]);
    this.kick();
    return batchId;
  }

  addDownload(alias: string, remotePath: string, kind: TransferKind, totalBytes: number): string {
    const id = identifier("transfer");
    const now = isoNow();
    const job: ManagedTransferJob = {
      id, batchId: identifier("batch"), alias, direction: "download", kind, name: baseName(remotePath),
      remotePath, totalBytes, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1,
      status: "queued", attempt: 1, problem: "", createdAt: now, updatedAt: now,
      batchName: baseName(remotePath), batchKind: kind, lastModified: 0, expectedRevision: "", overwrite: false,
    };
    this.downloadChunks.set(id, []);
    this.commit([...this.jobs, job]);
    this.kick();
    return id;
  }

  pause(id: string): void {
    const job = this.find(id);
    if (job === undefined || (job.status !== "running" && job.status !== "queued")) return;
    this.replace(id, { status: "paused" });
    this.controllers.get(id)?.abort();
    this.trackControl(id, this.api.updateTransfer(id, "pause").then(() => undefined).catch(() => undefined));
  }

  resume(id: string): void {
    const job = this.find(id);
    if (job === undefined || !["paused", "reattach", "needs_overwrite"].includes(job.status) || !networkReady(job, this.files)) return;
    if (job.direction === "download" && job.kind === "folder") {
      this.downloadChunks.set(id, []);
      this.replace(id, { transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1 });
    }
    this.replace(id, { status: "queued", problem: "" });
    this.kick();
  }

  retry(id: string): void {
    const job = this.find(id);
    if (job === undefined || job.status !== "failed" || !networkReady(job, this.files)) return;
    const reset = job.direction === "download" && job.kind === "folder";
    if (reset) this.downloadChunks.set(id, []);
    this.replace(id, {
      status: "queued", problem: "", attempt: job.attempt + 1, bytesPerSecond: 0, remainingSeconds: -1,
      ...(reset ? { transferredBytes: 0 } : {}),
    });
    this.kick();
  }

  retryFailed(batchId: string): void {
    for (const job of this.jobs) if (job.batchId === batchId && job.status === "failed") this.retry(job.id);
  }

  overwrite(id: string): void {
    const job = this.find(id);
    if (job === undefined || job.status !== "needs_overwrite" || !this.files.has(id)) return;
    this.replace(id, { overwrite: true, status: "queued", problem: "" });
    this.kick();
  }

  async cancel(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || job.status === "completed" || job.status === "cancelled") return;
    this.controllers.get(id)?.abort();
    this.replace(id, { status: "cancelled" });
    this.files.delete(id);
    this.downloadChunks.delete(id);
    await Promise.allSettled([
      this.api.updateTransfer(id, "cancel"),
      ...(job.direction === "upload" ? [this.api.cancelUpload(job.alias, id, job.remotePath)] : []),
    ]);
  }

  clearFinished(): void {
    this.commit(this.jobs.filter((job) => job.status !== "completed" && job.status !== "cancelled"));
  }

  dismissNotice(id: string): void {
    this.notices = this.notices.filter((notice) => notice.id !== id);
    for (const listener of this.noticeListeners) listener();
  }

  private kick(): void {
    while (this.active < this.concurrency) {
      const job = this.jobs.find((candidate) => candidate.status === "queued" && networkReady(candidate, this.files));
      if (job === undefined) return;
      this.active += 1;
      this.replace(job.id, { status: "running", updatedAt: isoNow() });
      this.samples.set(job.id, { at: this.now(), bytes: job.transferredBytes });
      void this.run(job.id).finally(() => {
        this.active -= 1;
        this.controllers.delete(job.id);
        this.kick();
      });
    }
  }

  private async run(id: string): Promise<void> {
    let job = this.find(id);
    if (job === undefined) return;
    try {
      if (!await this.prepareServer(job)) return;
      job = this.find(id);
      if (job === undefined || job.status !== "running") return;
      if (job.direction === "upload") await this.runUpload(id);
      else await this.runDownload(id);
    } catch (error) {
      job = this.find(id);
      if (job === undefined || job.status === "paused" || job.status === "cancelled") return;
      const code = failureCode(error) || (error instanceof Error ? error.message : "sftp_failed");
      if (code === "sftp_exists" && job.direction === "upload" && !job.overwrite) {
        await this.api.updateTransfer(id, "needs_overwrite").catch(() => undefined);
        this.replace(id, { status: "needs_overwrite", problem: "sftp_exists" });
        return;
      }
      await this.api.updateTransfer(id, "fail", { transferredBytes: job.transferredBytes, problem: code }).catch(() => undefined);
      this.replace(id, { status: "failed", problem: code });
      this.notify(this.find(id)!);
    }
  }

  private async prepareServer(job: ManagedTransferJob): Promise<boolean> {
    await this.controlOperations.get(job.id);
    const created = await this.api.createTransfer({
      id: job.id, batchId: job.batchId, alias: job.alias, direction: job.direction, kind: job.kind,
      name: job.name, remotePath: job.remotePath, totalBytes: job.totalBytes,
    });
    let status = created.status;
    const resetProgress = job.direction === "download" && job.kind === "folder" && job.transferredBytes === 0;
    if (status === "running") status = (await this.api.updateTransfer(job.id, "pause")).status;
    if (status === "paused" || status === "reattach" || status === "needs_overwrite") {
      status = (await this.api.updateTransfer(job.id, "resume", { resetProgress })).status;
    } else if (status === "failed") {
      status = (await this.api.updateTransfer(job.id, "retry", { resetProgress })).status;
    }
    if (status !== "queued") throw new Error("sftp_transfer_state");
    const current = this.find(job.id);
    if (current?.status !== "running") {
      await this.api.updateTransfer(job.id, current?.status === "cancelled" ? "cancel" : "pause").catch(() => undefined);
      return false;
    }
    await this.api.updateTransfer(job.id, "start");
    const afterStart = this.find(job.id);
    if (afterStart?.status !== "running") {
      await this.api.updateTransfer(job.id, afterStart?.status === "cancelled" ? "cancel" : "pause").catch(() => undefined);
      return false;
    }
    return true;
  }

  private async runUpload(id: string): Promise<void> {
    const file = this.files.get(id);
    let job = this.find(id);
    if (file === undefined || job === undefined) return;
    const started = await this.api.startUpload(job.alias, id, job.remotePath, job.totalBytes, job.overwrite, job.expectedRevision);
    this.replace(id, { transferredBytes: started.offset, expectedRevision: started.expectedRevision });
    let offset = started.offset;
    await this.api.updateTransfer(id, "progress", { transferredBytes: offset });
    while (offset < file.size) {
      job = this.find(id);
      if (job === undefined || job.status !== "running") return;
      const controller = new AbortController();
      this.controllers.set(id, controller);
      const end = Math.min(offset + chunkBytes, file.size);
      const appended = await this.retryOperation(() => this.api.appendUpload(job!.alias, id, job!.remotePath, offset, file.size, file.slice(offset, end), controller.signal));
      offset = appended.offset;
      this.recordProgress(id, offset, file.size);
      await this.api.updateTransfer(id, "progress", { transferredBytes: offset });
    }
    job = this.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.api.completeUpload(job.alias, id, job.remotePath, job.totalBytes, job.expectedRevision);
    await this.api.updateTransfer(id, "complete", { transferredBytes: job.totalBytes });
    this.files.delete(id);
    this.replace(id, { transferredBytes: job.totalBytes, status: "completed", remainingSeconds: 0 });
    this.notify(this.find(id)!);
  }

  private async runDownload(id: string): Promise<void> {
    let job = this.find(id);
    if (job === undefined) return;
    const chunks = this.downloadChunks.get(id) ?? [];
    this.downloadChunks.set(id, chunks);
    let failures = 0;
    while (true) {
      job = this.find(id);
      if (job === undefined || job.status !== "running") return;
      const controller = new AbortController();
      this.controllers.set(id, controller);
      try {
        await this.api.streamDownload(job.alias, job.remotePath, job.kind === "folder", job.transferredBytes, {
          signal: controller.signal,
          onChunk: (chunk, total) => {
            chunks.push(chunk);
            const current = this.find(id);
            if (current !== undefined) this.recordProgress(id, current.transferredBytes + chunk.byteLength, total ?? current.totalBytes);
          },
        });
        break;
      } catch (error) {
        const current = this.find(id);
        if (current === undefined || current.status !== "running" || controller.signal.aborted) throw error;
        if (current.kind === "folder" || failures >= 2) throw error;
        failures += 1;
      }
    }
    job = this.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.flushProgress(id);
    await this.api.updateTransfer(id, "progress", { transferredBytes: job.transferredBytes });
    this.api.saveDownload(job.remotePath, job.kind === "folder", chunks);
    await this.api.updateTransfer(id, "complete", { transferredBytes: job.transferredBytes });
    this.downloadChunks.delete(id);
    this.replace(id, { status: "completed", remainingSeconds: 0 });
    this.notify(this.find(id)!);
  }

  private recordProgress(id: string, transferredBytes: number, totalBytes: number): void {
    const job = this.find(id);
    if (job === undefined) return;
    const at = this.now();
    const sample = this.samples.get(id) ?? { at, bytes: job.transferredBytes };
    const elapsed = Math.max((at - sample.at) / 1000, 0);
    let bytesPerSecond = job.bytesPerSecond;
    if (elapsed > 0) {
      const instant = Math.max(0, transferredBytes - sample.bytes) / elapsed;
      bytesPerSecond = bytesPerSecond === 0 ? instant : bytesPerSecond * 0.65 + instant * 0.35;
      this.samples.set(id, { at, bytes: transferredBytes });
    }
    const resolvedTotal = totalBytes >= 0 ? totalBytes : job.totalBytes;
    const remainingSeconds = resolvedTotal >= 0 && bytesPerSecond > 0
      ? Math.max(0, Math.round((resolvedTotal - transferredBytes) / bytesPerSecond)) : -1;
    this.replace(id, { transferredBytes, totalBytes: resolvedTotal, bytesPerSecond, remainingSeconds, updatedAt: isoNow() });
    const reported = this.serverSamples.get(id) ?? { at: 0, bytes: 0 };
    if (transferredBytes-reported.bytes >= chunkBytes || at-reported.at >= 15_000) {
      this.serverSamples.set(id, { at, bytes: transferredBytes });
      const previous = this.progressReports.get(id) ?? Promise.resolve();
      const report = previous.catch(() => undefined)
        .then(async () => { await this.api.updateTransfer(id, "progress", { transferredBytes }); });
      this.progressReports.set(id, report);
    }
  }

  private async flushProgress(id: string): Promise<void> {
    await this.progressReports.get(id)?.catch(() => undefined);
    this.progressReports.delete(id);
  }

  private trackControl(id: string, operation: Promise<void>): void {
    this.controlOperations.set(id, operation);
    void operation.finally(() => {
      if (this.controlOperations.get(id) === operation) this.controlOperations.delete(id);
    });
  }

  private async retryOperation<T>(operation: () => Promise<T>): Promise<T> {
    let last: unknown;
    for (let attempt = 0; attempt < 3; attempt += 1) {
      try {
        return await operation();
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") throw error;
        last = error;
        if (attempt < 2) await new Promise((resolve) => globalThis.setTimeout(resolve, 250 * (2 ** attempt)));
      }
    }
    throw last;
  }

  private notify(job: ManagedTransferJob): void {
    if (job.status !== "completed" && job.status !== "failed") return;
    const notice: TransferNotice = {
      id: `${job.id}:${job.status}:${job.attempt}`, jobId: job.id, status: job.status,
      name: job.name, direction: job.direction, problem: job.problem,
    };
    if (this.notices.some((current) => current.id === notice.id)) return;
    this.notices = [...this.notices.slice(-7), notice];
    for (const listener of this.noticeListeners) listener();
  }

  private find(id: string): ManagedTransferJob | undefined {
    return this.jobs.find((job) => job.id === id);
  }

  private replace(id: string, changes: Partial<ManagedTransferJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  private commit(jobs: ManagedTransferJob[]): void {
    this.jobs = jobs;
    try {
      if (jobs.length === 0) globalThis.localStorage?.removeItem(persistenceKey);
      else globalThis.localStorage?.setItem(persistenceKey, JSON.stringify(jobs));
      globalThis.localStorage?.removeItem(legacyUploadKey);
    } catch {
      // In-memory transfers remain available when storage is disabled or full.
    }
    for (const listener of this.listeners) listener();
  }
}

export const sftpTransferManager = new SFTPTransferManager();

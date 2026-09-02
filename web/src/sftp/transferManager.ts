import { failureCode } from "../api/client";
import { notifyAndroidTransfer } from "../android/native";
import {
  sftpApi,
  type CreateTransferJob,
  type ResumableUpload,
  type StreamDownloadOptions,
  type TransferJob,
  type TransferJobAction,
  type TransferJobList,
  type TransferKind,
  type TransferQueueMove,
  type TransferSettings,
} from "./api";

const chunkBytes = 1 << 20;
const fallbackDownloadLimit = 64 << 20;
const maxTransferJobs = 200;
const defaultLargeFileThreshold = 100 << 20;
const defaultLargeFileParallelism = 4;
const defaultLargeFileChunkBytes = 32 << 20;

export type ManagedTransferJob = TransferJob;

export type UploadSelection = {
  alias: string;
  remotePath: string;
  localName: string;
  file: File;
};

export type RemoteTransferSelection = {
  sourceAlias: string;
  sourcePath: string;
  targetAlias: string;
  targetPath: string;
  kind: TransferKind;
  name: string;
  totalBytes: number;
  overwrite?: boolean;
};

export type UploadAdmission = {
  readonly count: number;
  release(): void;
};

export type TransferNotice = {
  id: string;
  jobId: string;
  status: "completed" | "failed";
  name: string;
  direction: "upload" | "download" | "remote";
  problem: string;
};

type ManagerAPI = {
  listTransfers(): Promise<TransferJobList>;
  updateTransferSettings(settings: TransferSettings): Promise<TransferJobList>;
  moveTransfer(id: string, move: TransferQueueMove): Promise<TransferJobList>;
  createTransfer(input: CreateTransferJob): Promise<TransferJob>;
  clearFinishedTransfers(): Promise<void>;
  removeTransfer(id: string): Promise<void>;
  updateTransfer(id: string, action: TransferJobAction, options?: { transferredBytes?: number; totalBytes?: number; problem?: string; resetProgress?: boolean }): Promise<TransferJob>;
  checkpointDownload(id: string, offset: number, revision: string): Promise<TransferJob>;
  verifyDownload(alias: string, jobId: string, remotePath: string, revision: string): Promise<void>;
  startUpload(alias: string, id: string, remotePath: string, size: number, sourceFingerprint: string): Promise<ResumableUpload>;
  appendUpload(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload>;
  appendUploadRange(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload>;
  completeUpload(alias: string, id: string, remotePath: string, size: number, expectedRevision: string, sourceFingerprint: string): Promise<void>;
  cancelUpload(alias: string, id: string, remotePath: string): Promise<void>;
  streamDownload(alias: string, jobId: string, remotePath: string, directory: boolean, offset: number, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }>;
  saveDownload(remotePath: string, directory: boolean, chunks: BlobPart[]): Promise<void> | void;
};

type DownloadSink = {
  root: FileSystemDirectoryHandle;
  name: string;
  handle: FileSystemFileHandle;
  writer: FileSystemWritableFileStream;
  position: number;
  reset: boolean;
};

function identifier(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function baseName(remotePath: string): string {
  const components = remotePath.split("/").filter(Boolean);
  return components[components.length - 1] ?? remotePath;
}

function networkReady(job: ManagedTransferJob, files: Map<string, File>): boolean {
  return job.direction === "remote" || job.direction === "download" || files.has(job.id);
}

function reattachableUpload(job: ManagedTransferJob, selection: UploadSelection, files: Map<string, File>): boolean {
  return job.direction === "upload" && !files.has(job.id) &&
    ["queued", "running", "paused", "reattach", "failed", "needs_overwrite"].includes(job.status) &&
    job.alias === selection.alias && job.remotePath === selection.remotePath &&
    job.totalBytes === selection.file.size && job.lastModified === selection.file.lastModified;
}

export class SFTPTransferManager {
  private jobs: ManagedTransferJob[] = [];
  private notices: TransferNotice[] = [];
  private readonly files = new Map<string, File>();
  private readonly downloadChunks = new Map<string, Uint8Array[]>();
  private readonly downloadSinks = new Map<string, DownloadSink>();
  private readonly controllers = new Map<string, AbortController>();
  private readonly inFlight = new Set<string>();
  private readonly uploadAdmissions = new Set<UploadAdmission>();
  private readonly samples = new Map<string, { at: number; bytes: number }>();
  private readonly controlOperations = new Map<string, Promise<void>>();
  private readonly retryAfter = new Map<string, number>();
  private readonly listeners = new Set<() => void>();
  private readonly noticeListeners = new Set<() => void>();
  private active = 0;
  private maxConcurrent: number;
  private clearCompletedAfter = 0;
  private processingStopped = false;
  private largeFileThreshold = defaultLargeFileThreshold;
  private largeFileParallelism = defaultLargeFileParallelism;
  private largeFileChunkBytes = defaultLargeFileChunkBytes;

  constructor(
    private readonly api: ManagerAPI = sftpApi,
    concurrency = 2,
    private readonly now = () => Date.now(),
  ) {
    this.maxConcurrent = concurrency;
  }

  getSnapshot = (): readonly ManagedTransferJob[] => this.jobs;
  getNoticeSnapshot = (): readonly TransferNotice[] => this.notices;
  getMaxConcurrent = (): number => this.maxConcurrent;
  getClearCompletedAfter = (): number => this.clearCompletedAfter;
  getProcessingStopped = (): boolean => this.processingStopped;
  getLargeFileThreshold = (): number => this.largeFileThreshold;
  getLargeFileParallelism = (): number => this.largeFileParallelism;
  getLargeFileChunkBytes = (): number => this.largeFileChunkBytes;
  hasUploadSource = (id: string): boolean => this.files.has(id);

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
    const serverIDs = new Set(listed.jobs.map((job) => job.id));
    if (listed.jobs.length > maxTransferJobs || serverIDs.size !== listed.jobs.length) {
      throw new Error("sftp_transfer_limit");
    }
    this.adoptSettings(listed);
    this.commit(listed.jobs);
    await this.cleanupOrphanDownloads(new Set(listed.jobs
      .filter((job) => !["completed", "cancelled"].includes(job.status))
      .map((job) => job.id)));
    this.kick();
  }

  // The queue belongs to the engine, so the settings do too: one value, shared
  // by every browser and every tab looking at the same engine.
  async applySettings(
    maxConcurrent: number,
    clearCompletedAfterSeconds: number,
    processingStopped: boolean,
    largeFileThresholdBytes: number,
    largeFileParallelism: number,
    largeFileChunkBytes: number,
  ): Promise<void> {
    const listed = await this.api.updateTransferSettings({
      maxConcurrent, clearCompletedAfterSeconds, processingStopped, largeFileThresholdBytes, largeFileParallelism, largeFileChunkBytes,
    });
    this.adoptSettings(listed);
    this.commit(listed.jobs);
    this.kick();
  }

  // Only waiting jobs move, and the engine decides what "waiting" means at the
  // moment the request lands.
  async move(id: string, move: TransferQueueMove): Promise<void> {
    const job = this.find(id);
    if (job === undefined || job.status !== "queued") return;
    const listed = await this.api.moveTransfer(id, move);
    this.adoptSettings(listed);
    this.commit(listed.jobs);
  }

  reserveUploads(selections: UploadSelection[]): UploadAdmission {
    const count = this.pendingUploadCount(selections);
    const reserved = [...this.uploadAdmissions].reduce((sum, admission) => sum + admission.count, 0);
    if (this.jobs.length + reserved + count > maxTransferJobs) throw new Error("sftp_transfer_limit");
    let released = false;
    const admission: UploadAdmission = {
      count,
      release: () => {
        if (released) return;
        released = true;
        this.uploadAdmissions.delete(admission);
      },
    };
    this.uploadAdmissions.add(admission);
    return admission;
  }

  async addUploads(selections: UploadSelection[], batch?: { id?: string; name: string; kind: TransferKind }, admission?: UploadAdmission): Promise<string> {
    const newSelections = selections.filter((selection) => !this.jobs.some((job) => reattachableUpload(job, selection, this.files)));
    const otherReserved = [...this.uploadAdmissions].reduce((sum, current) => sum + (current === admission ? 0 : current.count), 0);
    if (admission !== undefined && (!this.uploadAdmissions.has(admission) || newSelections.length > admission.count)) {
      throw new Error("sftp_transfer_limit");
    }
    if (this.jobs.length + otherReserved + newSelections.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
    admission?.release();
    const batchId = batch?.id ?? identifier("batch");
    const batchName = batch?.name ?? selections[0]?.localName ?? "upload";
    const batchKind = batch?.kind ?? (selections.length > 1 ? "folder" : "file");
    for (const selection of selections) {
      const existing = this.jobs.find((job) => reattachableUpload(job, selection, this.files));
      if (existing !== undefined) {
        this.files.set(existing.id, selection.file);
        if (existing.status === "running") {
          await this.api.updateTransfer(existing.id, "pause");
          const resumed = await this.api.updateTransfer(existing.id, "resume");
          this.replaceServer(resumed);
        } else if (existing.status === "paused" || existing.status === "reattach") {
          const resumed = await this.api.updateTransfer(existing.id, "resume");
          this.replaceServer(resumed);
        } else if (existing.status === "failed") {
          const retried = await this.api.updateTransfer(existing.id, "retry");
          this.replaceServer(retried);
        }
        continue;
      }
      const id = identifier("transfer");
      const job = await this.api.createTransfer({
        id, batchId, batchName, batchKind, alias: selection.alias, direction: "upload", kind: "file",
        name: selection.localName, remotePath: selection.remotePath, totalBytes: selection.file.size,
        lastModified: selection.file.lastModified,
        largeFileThresholdBytes: this.largeFileThreshold,
        largeFileParallelism: this.largeFileParallelism,
        largeFileChunkBytes: this.largeFileChunkBytes,
      });
      this.files.set(id, selection.file);
      this.commit([...this.jobs, job]);
    }
    this.kick();
    return batchId;
  }

  async addDownload(alias: string, remotePath: string, kind: TransferKind, totalBytes: number): Promise<string> {
    const reserved = [...this.uploadAdmissions].reduce((sum, admission) => sum + admission.count, 0);
    if (this.jobs.length + reserved >= maxTransferJobs) throw new Error("sftp_transfer_limit");
    const id = identifier("transfer");
    const name = baseName(remotePath);
    const job = await this.api.createTransfer({
      id, batchId: identifier("batch"), batchName: name, batchKind: kind, alias,
      direction: "download", kind, name, remotePath, totalBytes, lastModified: 0,
    });
    this.downloadChunks.set(id, []);
    this.commit([...this.jobs, job]);
    this.kick();
    return id;
  }

  async addRemoteTransfers(selections: RemoteTransferSelection[], operation: "copy" | "move"): Promise<string[]> {
    if (selections.length === 0) return [];
    const reserved = [...this.uploadAdmissions].reduce((sum, admission) => sum + admission.count, 0);
    if (this.jobs.length + reserved + selections.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
    const batchId = identifier("remote_batch");
    const batchName = selections.length === 1 ? selections[0]!.name : `${selections.length} remote items`;
    const batchKind: TransferKind = selections.length === 1 ? selections[0]!.kind : "folder";
    const ids: string[] = [];
    for (const selection of selections) {
      const id = identifier("remote");
      const job = await this.api.createTransfer({
        id, batchId, batchName, batchKind,
        alias: selection.targetAlias,
        sourceAlias: selection.sourceAlias,
        sourcePath: selection.sourcePath,
        operation,
        overwrite: selection.overwrite === true,
        direction: "remote",
        kind: selection.kind,
        name: selection.name,
        remotePath: selection.targetPath,
        totalBytes: selection.totalBytes,
        lastModified: 0,
      });
      this.commit([...this.jobs, job]);
      ids.push(id);
    }
    await this.reconcile();
    return ids;
  }

  async pause(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || !job.allowedActions.includes("pause")) return;
    this.controllers.get(id)?.abort();
    const operation = this.api.updateTransfer(id, "pause").then((updated) => { this.replaceServer(updated); });
    this.trackControl(id, operation);
    await operation;
  }

  async resume(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || !job.allowedActions.includes("resume") || !networkReady(job, this.files)) return;
    if (job.direction === "download" && job.kind === "folder") {
      this.downloadChunks.set(id, []);
      const sink = this.downloadSinks.get(id);
      if (sink !== undefined) sink.reset = true;
      this.replace(id, { transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1 });
    }
    const updated = await this.api.updateTransfer(id, "resume", { resetProgress: job.direction === "download" && job.kind === "folder" });
    this.replaceServer(updated);
    this.kick();
  }

  async retry(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || !job.allowedActions.includes("retry") || !networkReady(job, this.files)) return;
    const reset = job.direction === "download" && job.kind === "folder";
    if (reset) {
      this.downloadChunks.set(id, []);
      const sink = this.downloadSinks.get(id);
      if (sink !== undefined) sink.reset = true;
    }
    const updated = await this.api.updateTransfer(id, "retry", { resetProgress: reset });
    this.replaceServer(updated);
    this.kick();
  }

  async retryFailed(batchId: string): Promise<void> {
    await Promise.all(this.jobs.filter((job) => job.batchId === batchId && job.status === "failed").map((job) => this.retry(job.id)));
  }

  async pauseAll(): Promise<void> {
    await Promise.all([...this.jobs]
      .filter((job) => job.allowedActions.includes("pause"))
      .map((job) => this.pause(job.id)));
  }

  async resumeAll(): Promise<void> {
    await Promise.all([...this.jobs]
      .filter((job) => job.allowedActions.includes("resume") && job.status !== "needs_overwrite")
      .map((job) => this.resume(job.id)));
  }

  async cancelAll(): Promise<void> {
    let firstFailure: unknown;
    for (const job of [...this.jobs]) {
      if (!this.find(job.id)?.allowedActions.includes("cancel")) continue;
      try {
        await this.cancel(job.id);
      } catch (error) {
        firstFailure ??= error;
      }
    }
    if (firstFailure !== undefined) throw firstFailure;
  }

  async overwrite(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || job.status !== "needs_overwrite" || !this.files.has(id)) return;
    await this.resume(id);
  }

  async cancel(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || !job.allowedActions.includes("cancel")) return;
    this.controllers.get(id)?.abort();
    if (job.direction === "upload") {
      try {
        await this.api.cancelUpload(job.alias, id, job.remotePath);
      } catch (error) {
        if (missingServerTransfer(error)) {
          this.files.delete(id);
          await this.reconcile();
          return;
        }
        await this.reconcile().catch(() => undefined);
        throw error;
      }
      this.files.delete(id);
    } else {
      try {
        const updated = await this.api.updateTransfer(id, "cancel");
        this.replaceServer(updated);
      } catch (error) {
        if (!missingServerTransfer(error)) throw error;
      }
      this.downloadChunks.delete(id);
      await this.cleanupDownload(id);
    }
    await this.reconcile();
  }

  async clearFinished(): Promise<void> {
    const removed = this.jobs.filter((job) => job.status === "completed" || job.status === "cancelled").map((job) => job.id);
    await this.api.clearFinishedTransfers();
    this.commit(this.jobs.filter((job) => job.status !== "completed" && job.status !== "cancelled"));
    for (const id of removed) void this.cleanupDownload(id);
  }

  async remove(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || !job.allowedActions.includes("remove")) return;
    await this.api.removeTransfer(id);
    this.files.delete(id);
    this.downloadChunks.delete(id);
    await this.cleanupDownload(id);
    this.commit(this.jobs.filter((candidate) => candidate.id !== id));
  }

  async clearFailed(): Promise<void> {
    let firstFailure: unknown;
    for (const job of [...this.jobs]) {
      if (job.status !== "failed" || !job.allowedActions.includes("remove")) continue;
      try {
        await this.remove(job.id);
      } catch (error) {
        firstFailure ??= error;
      }
    }
    if (firstFailure !== undefined) throw firstFailure;
  }

  dismissNotice(id: string): void {
    this.notices = this.notices.filter((notice) => notice.id !== id);
    for (const listener of this.noticeListeners) listener();
  }

  private adoptSettings(listed: TransferJobList): void {
    this.maxConcurrent = listed.maxConcurrent;
    this.clearCompletedAfter = listed.clearCompletedAfterSeconds ?? 0;
    this.processingStopped = listed.processingStopped === true;
    this.largeFileThreshold = listed.largeFileThresholdBytes ?? defaultLargeFileThreshold;
    this.largeFileParallelism = listed.largeFileParallelism ?? defaultLargeFileParallelism;
    this.largeFileChunkBytes = listed.largeFileChunkBytes ?? defaultLargeFileChunkBytes;
  }

  private kick(): void {
    // The engine refuses a start while the queue is stopped. Asking anyway
    // would just spend a request per waiting job on every tick.
    if (this.processingStopped) return;
    while (this.active < this.maxConcurrent) {
      const job = this.jobs.find((candidate) => candidate.status === "queued" &&
        candidate.direction !== "remote" &&
        !this.inFlight.has(candidate.id) && (this.retryAfter.get(candidate.id) ?? 0) <= this.now() &&
        networkReady(candidate, this.files));
      if (job === undefined) return;
      this.active += 1;
      this.inFlight.add(job.id);
      this.samples.set(job.id, { at: this.now(), bytes: job.transferredBytes });
      void this.run(job.id).finally(() => {
        this.active -= 1;
        this.inFlight.delete(job.id);
        this.controllers.delete(job.id);
        this.kick();
      });
    }
  }

  private async run(id: string): Promise<void> {
    let job = this.find(id);
    if (job === undefined) return;
    try {
      let sourceFingerprint = "";
      if (job.direction === "upload") {
        const file = this.files.get(id);
        if (file === undefined) return;
        const controller = new AbortController();
        this.controllers.set(id, controller);
        sourceFingerprint = await fingerprintFile(file, controller.signal, () => this.find(id)?.status === "queued");
        const current = this.find(id);
        if (current === undefined || current.status !== "queued") return;
        if (current.sourceFingerprint && current.sourceFingerprint !== sourceFingerprint) {
          throw new Error("sftp_upload_source_changed");
        }
        job = this.find(id)!;
      }
      if (!await this.prepareServer(job)) return;
      job = this.find(id);
      if (job === undefined || job.status !== "running") return;
      if (job.direction === "upload") await this.runUpload(id, sourceFingerprint);
      else await this.runDownload(id);
    } catch (error) {
      job = this.find(id);
      if (job === undefined || job.status === "paused" || job.status === "cancelled" ||
          (error instanceof DOMException && error.name === "AbortError")) return;
      const code = failureCode(error) || (error instanceof Error ? error.message : "sftp_failed");
      if (code === "sftp_transfer_limit" || code === "sftp_transfer_state") {
        this.retryAfter.set(id, this.now() + 500);
        await this.reconcile().catch(() => undefined);
        globalThis.setTimeout(() => {
          this.retryAfter.delete(id);
          this.kick();
        }, 500);
        return;
      }
      if (code === "sftp_exists" && job.direction === "upload" && !job.overwrite) {
        const updated = await this.api.updateTransfer(id, "needs_overwrite").catch(() => null);
        if (updated !== null) this.replaceServer(updated);
        return;
      }
      const failed = await this.api.updateTransfer(id, "fail", { problem: code }).catch(() => null);
      if (failed !== null) {
        this.replaceServer(failed);
        this.notify(failed);
      } else {
        await this.reconcile().catch(() => undefined);
      }
    }
  }

  private async prepareServer(job: ManagedTransferJob): Promise<boolean> {
    await this.controlOperations.get(job.id);
    const current = this.find(job.id);
    if (current?.status !== "queued") return false;
    const started = await this.api.updateTransfer(job.id, "start");
    this.retryAfter.delete(job.id);
    this.replaceServer(started);
    return started.status === "running";
  }

  private async runUpload(id: string, sourceFingerprint: string): Promise<void> {
    const file = this.files.get(id);
    let job = this.find(id);
    if (file === undefined || job === undefined) return;
    const started = await this.api.startUpload(job.alias, id, job.remotePath, job.totalBytes, sourceFingerprint);
    this.replace(id, { transferredBytes: started.offset, expectedRevision: started.expectedRevision });
    if (started.parallelism > 1) {
      await this.runParallelUpload(id, file, started, sourceFingerprint);
      return;
    }
    let offset = started.offset;
    while (offset < file.size) {
      job = this.find(id);
      if (job === undefined || job.status !== "running") return;
      const controller = new AbortController();
      this.controllers.set(id, controller);
      const end = Math.min(offset + chunkBytes, file.size);
      const appended = await this.retryOperation(() => this.api.appendUpload(job!.alias, id, job!.remotePath, offset, file.size, file.slice(offset, end), controller.signal));
      offset = appended.offset;
      this.recordProgress(id, offset, file.size);
    }
    job = this.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.api.completeUpload(job.alias, id, job.remotePath, job.totalBytes, job.expectedRevision, sourceFingerprint);
    this.files.delete(id);
    await this.reconcile();
    const completed = this.find(id);
    if (completed !== undefined) this.notify(completed);
  }

  private async runParallelUpload(id: string, file: File, started: ResumableUpload, sourceFingerprint: string): Promise<void> {
    const ranges: Array<{ offset: number; size: number }> = [];
    for (let offset = 0; offset < file.size; offset += started.chunkBytes) {
      const size = Math.min(started.chunkBytes, file.size - offset);
      if (!started.completedRanges.some((range) => offset >= range.offset && offset + size <= range.offset + range.size)) {
        ranges.push({ offset, size });
      }
    }
    const controller = new AbortController();
    this.controllers.set(id, controller);
    let next = 0;
    const worker = async () => {
      while (next < ranges.length) {
        const portion = ranges[next++];
        if (portion === undefined) return;
        const job = this.find(id);
        if (job === undefined || job.status !== "running") return;
        const appended = await this.retryOperation(() => this.api.appendUploadRange(
          job.alias, id, job.remotePath, portion.offset, file.size,
          file.slice(portion.offset, portion.offset + portion.size), controller.signal,
        ));
        const current = this.find(id)?.transferredBytes ?? 0;
        this.recordProgress(id, Math.max(current, appended.offset), file.size);
      }
    };
    await Promise.all(Array.from({ length: Math.min(started.parallelism, ranges.length) }, worker));
    let job = this.find(id);
    if (job === undefined || job.status !== "running") return;
    await this.api.completeUpload(job.alias, id, job.remotePath, job.totalBytes, started.expectedRevision, sourceFingerprint);
    this.files.delete(id);
    await this.reconcile();
    job = this.find(id);
    if (job !== undefined) this.notify(job);
  }

  private async runDownload(id: string): Promise<void> {
    let job = this.find(id);
    if (job === undefined) return;
    const sink = await this.openDownloadSink(job);
    const chunks = this.downloadChunks.get(id) ?? [];
    this.downloadChunks.set(id, chunks);
    let bufferedBytes = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0);

    if (sink !== null && sink.position !== job.transferredBytes && job.downloadRevision !== "") {
      try {
        await this.api.checkpointDownload(id, sink.position, job.downloadRevision);
        this.replace(id, { transferredBytes: sink.position });
        job = this.find(id)!;
      } catch {
        await sink.writer.truncate(0);
        await sink.writer.seek(0);
        sink.position = 0;
        sink.reset = true;
      }
    }
    const lostFallbackPrefix = sink === null && job.transferredBytes > 0 &&
      (bufferedBytes !== job.transferredBytes || job.downloadRevision === "");
    if (lostFallbackPrefix || sink?.reset === true) {
      chunks.length = 0;
      bufferedBytes = 0;
      if (sink !== null) sink.reset = false;
      this.replace(id, { transferredBytes: 0, downloadRevision: "", bytesPerSecond: 0, remainingSeconds: -1 });
      await this.api.updateTransfer(id, "progress", { transferredBytes: 0, resetProgress: true });
      job = this.find(id);
      if (job === undefined) return;
    }

    const alreadyComplete = sink !== null && job.kind === "file" && job.downloadRevision !== "" &&
      job.totalBytes >= 0 && sink.position === job.totalBytes;
    let shouldStream = !alreadyComplete;
    if (alreadyComplete) {
      try {
        await this.api.verifyDownload(job.alias, id, job.remotePath, job.downloadRevision);
        await this.api.checkpointDownload(id, sink.position, job.downloadRevision);
        this.replace(id, { transferredBytes: sink.position });
        job = this.find(id)!;
      } catch {
        await sink.writer.truncate(0);
        await sink.writer.seek(0);
        sink.position = 0;
        await this.checkpointDownloadSink(sink);
        this.replace(id, { transferredBytes: 0, downloadRevision: "", bytesPerSecond: 0, remainingSeconds: -1 });
        await this.api.updateTransfer(id, "progress", { transferredBytes: 0, resetProgress: true });
        job = this.find(id)!;
        shouldStream = true;
      }
    }
    if (shouldStream) {
      let failures = 0;
      let responseRevision = job.downloadRevision;
      while (true) {
        job = this.find(id);
        if (job === undefined || job.status !== "running") return;
        const controller = new AbortController();
        this.controllers.set(id, controller);
        try {
          await this.api.streamDownload(job.alias, id, job.remotePath, job.kind === "folder", job.transferredBytes, {
            ...(job.downloadRevision === "" ? {} : { revision: job.downloadRevision }),
            signal: controller.signal,
            onRevision: (revision) => {
              responseRevision = revision;
              this.replace(id, { downloadRevision: revision });
            },
            onReset: async (total) => {
              chunks.length = 0;
              bufferedBytes = 0;
              if (sink !== null) {
                await sink.writer.truncate(0);
                await sink.writer.seek(0);
                sink.position = 0;
                await this.checkpointDownloadSink(sink);
              }
              const totalBytes = total ?? this.find(id)?.totalBytes ?? -1;
              this.replace(id, { transferredBytes: 0, totalBytes, bytesPerSecond: 0, remainingSeconds: -1 });
            },
            onChunk: async (chunk, total) => {
              if (responseRevision === "") throw new Error("download_revision_missing");
              let position: number;
              if (sink !== null) {
                await sink.writer.write(new Uint8Array(chunk));
                sink.position += chunk.byteLength;
                await this.checkpointDownloadSink(sink);
                position = sink.position;
              } else {
                if (bufferedBytes + chunk.byteLength > fallbackDownloadLimit) {
                  throw new Error("sftp_download_storage_unsupported");
                }
                chunks.push(chunk);
                bufferedBytes += chunk.byteLength;
                position = bufferedBytes;
              }
              const acknowledged = await this.api.checkpointDownload(id, position, responseRevision);
              const current = this.find(id);
              if (current !== undefined) {
                this.recordProgress(id, position, total ?? acknowledged.totalBytes);
              }
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
    }
    job = this.find(id);
    if (job === undefined || job.status !== "running") return;
    if (sink !== null) {
      await sink.writer.close();
      this.downloadSinks.delete(id);
      await this.api.saveDownload(job.remotePath, job.kind === "folder", [await sink.handle.getFile()]);
      globalThis.setTimeout(() => { void sink.root.removeEntry(sink.name).catch(() => undefined); }, 30_000);
    } else {
      await this.api.saveDownload(job.remotePath, job.kind === "folder", chunks.map((chunk) => new Uint8Array(chunk)));
      if (job.downloadRevision === "") throw new Error("download_revision_missing");
      const acknowledged = await this.api.checkpointDownload(id, bufferedBytes, job.downloadRevision);
      this.recordProgress(id, bufferedBytes, acknowledged.totalBytes >= 0 ? acknowledged.totalBytes : bufferedBytes);
      job = this.find(id)!;
    }
    const completed = await this.api.updateTransfer(id, "complete");
    this.downloadChunks.delete(id);
    this.replaceServer(completed);
    this.notify(completed);
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
    this.replace(id, { transferredBytes, totalBytes: resolvedTotal, bytesPerSecond, remainingSeconds });
  }

  private async openDownloadSink(job: ManagedTransferJob): Promise<DownloadSink | null> {
    const existing = this.downloadSinks.get(job.id);
    if (existing !== undefined) return existing;
    try {
      if (typeof globalThis.navigator?.storage?.getDirectory !== "function") return null;
      const root = await globalThis.navigator.storage.getDirectory();
      const name = `sshc-sftp-${job.id}.part`;
      const handle = await root.getFileHandle(name, { create: true });
      const writer = await handle.createWritable({ keepExistingData: true });
      const storedSize = (await handle.getFile()).size;
      const invalidStoredSize = storedSize < 0 || (job.totalBytes >= 0 && storedSize > job.totalBytes);
      const reset = invalidStoredSize || (storedSize !== job.transferredBytes && job.downloadRevision === "");
      if (reset) await writer.truncate(0);
      const position = reset ? 0 : storedSize;
      await writer.seek(position);
      const sink = { root, name, handle, writer, reset, position };
      this.downloadSinks.set(job.id, sink);
      return sink;
    } catch {
      return null;
    }
  }

  private async cleanupDownload(id: string): Promise<void> {
    const sink = this.downloadSinks.get(id);
    this.downloadSinks.delete(id);
    if (sink !== undefined) {
      await sink.writer.close().catch(() => undefined);
      await sink.root.removeEntry(sink.name).catch(() => undefined);
      return;
    }
    try {
      const root = await globalThis.navigator.storage.getDirectory();
      await root.removeEntry(`sshc-sftp-${id}.part`).catch(() => undefined);
    } catch { /* OPFS is optional. */ }
  }

  private async cleanupOrphanDownloads(active: Set<string>): Promise<void> {
    try {
      if (typeof globalThis.navigator?.storage?.getDirectory !== "function") return;
      const root = await globalThis.navigator.storage.getDirectory();
      const entries = (root as unknown as { values(): AsyncIterable<FileSystemHandle> }).values();
      for await (const entry of entries) {
        const match = /^sshc-sftp-(.+)\.part$/.exec(entry.name);
        const id = match?.[1];
        if (id !== undefined && !active.has(id)) await root.removeEntry(entry.name).catch(() => undefined);
      }
    } catch { /* OPFS is optional. */ }
  }

  private async checkpointDownloadSink(sink: DownloadSink): Promise<void> {
    await sink.writer.close();
    const committed = await sink.handle.getFile();
    if (committed.size !== sink.position) throw new Error("sftp_download_checkpoint_failed");
    sink.writer = await sink.handle.createWritable({ keepExistingData: true });
    await sink.writer.seek(sink.position);
  }

  private trackControl(id: string, operation: Promise<void>): void {
    this.controlOperations.set(id, operation);
    const settled = () => {
      if (this.controlOperations.get(id) === operation) this.controlOperations.delete(id);
    };
    void operation.then(settled, settled);
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
    notifyAndroidTransfer(job.status);
    for (const listener of this.noticeListeners) listener();
  }

  private find(id: string): ManagedTransferJob | undefined {
    return this.jobs.find((job) => job.id === id);
  }

  private pendingUploadCount(selections: UploadSelection[]): number {
    return selections.filter((selection) => !this.jobs.some((job) => reattachableUpload(job, selection, this.files))).length;
  }

  private replace(id: string, changes: Partial<ManagedTransferJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  private replaceServer(updated: TransferJob): void {
    this.commit(this.jobs.some((job) => job.id === updated.id)
      ? this.jobs.map((job) => job.id === updated.id ? updated : job)
      : [...this.jobs, updated]);
  }

  private commit(jobs: ManagedTransferJob[]): void {
    this.jobs = jobs;
    for (const listener of this.listeners) listener();
  }
}

function missingServerTransfer(error: unknown): boolean {
  const code = failureCode(error) || (error instanceof Error ? error.message : "");
  return code === "sftp_transfer_not_found";
}

async function fingerprintFile(file: File, signal?: AbortSignal, running: () => boolean = () => true): Promise<string> {
  const chunkHashes: Uint8Array[] = [];
  for (let offset = 0; offset < file.size; offset += chunkBytes) {
    if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
    chunkHashes.push(new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", await file.slice(offset, offset + chunkBytes).arrayBuffer())));
    if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
  }
  const summary = new Uint8Array(8 + chunkHashes.length * 32);
  const sizeView = new DataView(summary.buffer);
  sizeView.setUint32(0, Math.floor(file.size / 0x1_0000_0000), false);
  sizeView.setUint32(4, file.size % 0x1_0000_0000, false);
  chunkHashes.forEach((hash, index) => summary.set(hash, 8 + index * 32));
  const digest = new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", summary));
  if (signal?.aborted || !running()) throw new DOMException("Transfer interrupted", "AbortError");
  return `tree-sha256:${[...digest].map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

export const sftpTransferManager = new SFTPTransferManager();

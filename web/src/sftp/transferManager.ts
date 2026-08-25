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
const fallbackDownloadLimit = 64 << 20;
const maxTransferJobs = 200;
const persistenceKey = "sshc.sftp.transfer-manager.v3";

export type ManagedTransferJob = TransferJob & {
  batchName: string;
  batchKind: TransferKind;
  lastModified: number;
  expectedRevision: string;
  downloadRevision: string;
  sourceFingerprint: string;
  overwrite: boolean;
};

export type UploadSelection = {
  alias: string;
  remotePath: string;
  localName: string;
  file: File;
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
  direction: "upload" | "download";
  problem: string;
};

type ManagerAPI = {
  listTransfers(): Promise<{ maxConcurrent: number; jobs: TransferJob[] }>;
  createTransfer(input: CreateTransferJob): Promise<TransferJob>;
  updateTransfer(id: string, action: TransferJobAction, options?: { transferredBytes?: number; totalBytes?: number; problem?: string; resetProgress?: boolean }): Promise<TransferJob>;
  checkpointDownload(id: string, offset: number, revision: string): Promise<TransferJob>;
  verifyDownload(alias: string, jobId: string, remotePath: string, revision: string): Promise<void>;
  startUpload(alias: string, id: string, remotePath: string, size: number, overwrite: boolean, expectedRevision?: string): Promise<ResumableUpload>;
  appendUpload(alias: string, id: string, remotePath: string, offset: number, total: number, chunk: Blob, signal?: AbortSignal): Promise<ResumableUpload>;
  completeUpload(alias: string, id: string, remotePath: string, size: number, expectedRevision: string, sourceFingerprint: string): Promise<void>;
  cancelUpload(alias: string, id: string, remotePath: string): Promise<void>;
  streamDownload(alias: string, jobId: string, remotePath: string, directory: boolean, offset: number, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }>;
  saveDownload(remotePath: string, directory: boolean, chunks: BlobPart[]): void;
};

type DownloadSink = {
  root: FileSystemDirectoryHandle;
  name: string;
  handle: FileSystemFileHandle;
  writer: FileSystemWritableFileStream;
  position: number;
  reset: boolean;
};

type StoredJob = Omit<ManagedTransferJob, "bytesPerSecond" | "remainingSeconds">;

function identifier(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function isoNow(): string {
  return new Date().toISOString();
}

function baseName(remotePath: string): string {
  const components = remotePath.split("/").filter(Boolean);
  return components[components.length - 1] ?? remotePath;
}

function restorable(value: unknown): ManagedTransferJob[] {
  if (!Array.isArray(value)) return [];
  if (value.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
  return value.flatMap((candidate) => {
    if (candidate === null || typeof candidate !== "object") return [];
    const job = candidate as Partial<StoredJob>;
    if (typeof job.id !== "string" || typeof job.batchId !== "string" || typeof job.alias !== "string" ||
        (job.direction !== "upload" && job.direction !== "download") || (job.kind !== "file" && job.kind !== "folder") ||
        typeof job.name !== "string" || typeof job.remotePath !== "string" || typeof job.totalBytes !== "number" ||
        typeof job.transferredBytes !== "number" || typeof job.status !== "string" || typeof job.attempt !== "number" ||
        typeof job.batchName !== "string" || (job.batchKind !== "file" && job.batchKind !== "folder") ||
        typeof job.lastModified !== "number" || typeof job.expectedRevision !== "string" ||
        typeof job.downloadRevision !== "string" || typeof job.sourceFingerprint !== "string" ||
        typeof job.overwrite !== "boolean") return [];
    let status = job.status;
    let transferredBytes = job.transferredBytes;
    if (!['completed', 'cancelled', 'failed', 'needs_overwrite'].includes(status)) status = "reattach";
    let downloadRevision = job.downloadRevision;
    if (job.direction === "download" && job.kind === "folder" && !["completed", "cancelled"].includes(status) &&
        (transferredBytes > 0 || downloadRevision !== "")) {
      status = "queued";
      transferredBytes = 0;
      downloadRevision = "";
    }
    return [{
      ...job as StoredJob, status, transferredBytes, downloadRevision, bytesPerSecond: 0, remainingSeconds: -1,
      problem: typeof job.problem === "string" ? job.problem : "",
      createdAt: typeof job.createdAt === "string" ? job.createdAt : isoNow(),
      updatedAt: typeof job.updatedAt === "string" ? job.updatedAt : isoNow(),
    }];
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
  private readonly downloadSinks = new Map<string, DownloadSink>();
  private readonly controllers = new Map<string, AbortController>();
  private readonly inFlight = new Set<string>();
  private readonly uploadAdmissions = new Set<UploadAdmission>();
  private readonly samples = new Map<string, { at: number; bytes: number }>();
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
    } catch {
      this.jobs = [];
      // Keep an invalid document untouched until server reconciliation can
      // replace it. Clearing it here would turn a bounded-load refusal into
      // irreversible, silent loss of the only local transfer metadata.
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
    await this.cleanupOrphanDownloads(new Set(this.jobs
      .filter((job) => !["completed", "cancelled"].includes(job.status))
      .map((job) => job.id)));
    const listed = await this.api.listTransfers();
    const serverIDs = new Set(listed.jobs.map((job) => job.id));
    if (listed.jobs.length > maxTransferJobs || serverIDs.size !== listed.jobs.length) {
      throw new Error("sftp_transfer_limit");
    }
    const localOnly = this.jobs.filter((job) => !serverIDs.has(job.id));
    const localCapacity = maxTransferJobs - listed.jobs.length;
    const retainedLocalIDs = new Set(localOnly
      .map((job, index) => ({ job, index }))
      .sort((left, right) => reconcilePriority(left.job) - reconcilePriority(right.job) || left.index - right.index)
      .slice(0, localCapacity)
      .map(({ job }) => job.id));
    const merged = this.jobs.filter((job) => serverIDs.has(job.id) || retainedLocalIDs.has(job.id));
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
      }
      const restored: ManagedTransferJob = {
        ...serverJob, status, transferredBytes,
        batchName: local?.batchName ?? serverJob.name, batchKind: local?.batchKind ?? serverJob.kind,
        lastModified: local?.lastModified ?? 0, expectedRevision: local?.expectedRevision ?? "",
        downloadRevision: local?.downloadRevision ?? "", sourceFingerprint: local?.sourceFingerprint ?? "",
        overwrite: local?.overwrite ?? false,
      };
      if (restored.direction === "download" && restored.kind === "folder" && local?.status === "queued" &&
          !["completed", "cancelled"].includes(restored.status)) {
        restored.status = "queued";
        restored.transferredBytes = 0;
        restored.downloadRevision = "";
      }
      if (index < 0) merged.push(restored);
      else merged[index] = restored;
    }
    this.commit(merged);
    this.kick();
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

  addUploads(selections: UploadSelection[], batch?: { id?: string; name: string; kind: TransferKind }, admission?: UploadAdmission): string {
    const newSelections = selections.filter((selection) => !this.jobs.some((job) => job.direction === "upload" &&
      job.alias === selection.alias && job.remotePath === selection.remotePath && job.totalBytes === selection.file.size &&
      job.lastModified === selection.file.lastModified && job.status === "reattach"));
    const otherReserved = [...this.uploadAdmissions].reduce((sum, current) => sum + (current === admission ? 0 : current.count), 0);
    if (admission !== undefined && (!this.uploadAdmissions.has(admission) || newSelections.length > admission.count)) {
      throw new Error("sftp_transfer_limit");
    }
    if (this.jobs.length + otherReserved + newSelections.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
    admission?.release();
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
        downloadRevision: "", sourceFingerprint: "",
      };
      this.files.set(id, selection.file);
      additions.push(job);
    }
    if (additions.length > 0) this.commit([...this.jobs, ...additions]);
    this.kick();
    return batchId;
  }

  addDownload(alias: string, remotePath: string, kind: TransferKind, totalBytes: number): string {
    const reserved = [...this.uploadAdmissions].reduce((sum, admission) => sum + admission.count, 0);
    if (this.jobs.length + reserved >= maxTransferJobs) throw new Error("sftp_transfer_limit");
    const id = identifier("transfer");
    const now = isoNow();
    const job: ManagedTransferJob = {
      id, batchId: identifier("batch"), alias, direction: "download", kind, name: baseName(remotePath),
      remotePath, totalBytes, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1,
      status: "queued", attempt: 1, problem: "", createdAt: now, updatedAt: now,
      batchName: baseName(remotePath), batchKind: kind, lastModified: 0, expectedRevision: "", overwrite: false,
      downloadRevision: "", sourceFingerprint: "",
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
      const sink = this.downloadSinks.get(id);
      if (sink !== undefined) sink.reset = true;
      this.replace(id, { transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1 });
    }
    this.replace(id, { status: "queued", problem: "" });
    this.kick();
  }

  retry(id: string): void {
    const job = this.find(id);
    if (job === undefined || job.status !== "failed" || !networkReady(job, this.files)) return;
    const reset = job.direction === "download" && job.kind === "folder";
    if (reset) {
      this.downloadChunks.set(id, []);
      const sink = this.downloadSinks.get(id);
      if (sink !== undefined) sink.reset = true;
    }
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
    if (job.direction === "upload") {
      try {
        await this.api.cancelUpload(job.alias, id, job.remotePath);
      } catch (error) {
        if (missingServerTransfer(error)) {
          this.files.delete(id);
          this.replace(id, { status: "cancelled", problem: "" });
          return;
        }
        this.replace(id, { status: "failed", problem: "sftp_cleanup_pending" });
        return;
      }
      this.files.delete(id);
    } else {
      try {
        await this.api.updateTransfer(id, "cancel");
      } catch (error) {
        if (!missingServerTransfer(error)) throw error;
      }
      this.downloadChunks.delete(id);
      await this.cleanupDownload(id);
    }
    this.replace(id, { status: "cancelled", problem: "" });
  }

  clearFinished(): void {
    const removed = this.jobs.filter((job) => job.status === "completed" || job.status === "cancelled").map((job) => job.id);
    this.commit(this.jobs.filter((job) => job.status !== "completed" && job.status !== "cancelled"));
    for (const id of removed) void this.cleanupDownload(id);
  }

  dismissNotice(id: string): void {
    this.notices = this.notices.filter((notice) => notice.id !== id);
    for (const listener of this.noticeListeners) listener();
  }

  private kick(): void {
    while (this.active < this.concurrency) {
      const job = this.jobs.find((candidate) => candidate.status === "queued" &&
        !this.inFlight.has(candidate.id) && networkReady(candidate, this.files));
      if (job === undefined) return;
      this.active += 1;
      this.inFlight.add(job.id);
      this.replace(job.id, { status: "running", updatedAt: isoNow() });
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
        sourceFingerprint = await fingerprintFile(file, controller.signal, () => this.find(id)?.status === "running");
        const current = this.find(id);
        if (current === undefined || current.status !== "running") return;
        if (current.sourceFingerprint !== "" && current.sourceFingerprint !== sourceFingerprint) {
          throw new Error("sftp_upload_source_changed");
        }
        this.replace(id, { sourceFingerprint });
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
      if (code === "sftp_exists" && job.direction === "upload" && !job.overwrite) {
        await this.api.updateTransfer(id, "needs_overwrite").catch(() => undefined);
        this.replace(id, { status: "needs_overwrite", problem: "sftp_exists" });
        return;
      }
      await this.api.updateTransfer(id, "fail", { problem: code }).catch(() => undefined);
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
    if (status === "completed") {
      this.files.delete(job.id);
      this.replace(job.id, { status: "completed", transferredBytes: created.totalBytes, remainingSeconds: 0 });
      this.notify(this.find(job.id)!);
      return false;
    }
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
    if (job.direction === "download" && job.kind === "folder") {
      await this.api.updateTransfer(job.id, "progress", { transferredBytes: 0, resetProgress: true });
      this.replace(job.id, { transferredBytes: 0, downloadRevision: "" });
    }
    return true;
  }

  private async runUpload(id: string, sourceFingerprint: string): Promise<void> {
    const file = this.files.get(id);
    let job = this.find(id);
    if (file === undefined || job === undefined) return;
    const started = await this.api.startUpload(job.alias, id, job.remotePath, job.totalBytes, job.overwrite, job.expectedRevision);
    this.replace(id, { transferredBytes: started.offset, expectedRevision: started.expectedRevision });
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
    this.replace(id, { transferredBytes: job.totalBytes, status: "completed", remainingSeconds: 0 });
    this.notify(this.find(id)!);
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
      this.api.saveDownload(job.remotePath, job.kind === "folder", [await sink.handle.getFile()]);
      globalThis.setTimeout(() => { void sink.root.removeEntry(sink.name).catch(() => undefined); }, 30_000);
    } else {
      this.api.saveDownload(job.remotePath, job.kind === "folder", chunks.map((chunk) => new Uint8Array(chunk)));
      if (job.downloadRevision === "") throw new Error("download_revision_missing");
      const acknowledged = await this.api.checkpointDownload(id, bufferedBytes, job.downloadRevision);
      this.recordProgress(id, bufferedBytes, acknowledged.totalBytes >= 0 ? acknowledged.totalBytes : bufferedBytes);
      job = this.find(id)!;
    }
    await this.api.updateTransfer(id, "complete");
    this.downloadChunks.delete(id);
    this.replace(id, { status: "completed", remainingSeconds: 0, downloadRevision: "" });
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

  private pendingUploadCount(selections: UploadSelection[]): number {
    return selections.filter((selection) => !this.jobs.some((job) => job.direction === "upload" &&
      job.alias === selection.alias && job.remotePath === selection.remotePath && job.totalBytes === selection.file.size &&
      job.lastModified === selection.file.lastModified && job.status === "reattach")).length;
  }

  private replace(id: string, changes: Partial<ManagedTransferJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  private commit(jobs: ManagedTransferJob[]): void {
    this.jobs = jobs;
    try {
      if (jobs.length === 0) globalThis.localStorage?.removeItem(persistenceKey);
      else globalThis.localStorage?.setItem(persistenceKey, JSON.stringify(jobs));
    } catch {
      // In-memory transfers remain available when storage is disabled or full.
    }
    for (const listener of this.listeners) listener();
  }
}

function missingServerTransfer(error: unknown): boolean {
  const code = failureCode(error) || (error instanceof Error ? error.message : "");
  return code === "sftp_transfer_not_found";
}

function reconcilePriority(job: ManagedTransferJob): number {
  if (job.direction === "upload" && job.problem === "sftp_cleanup_pending") return 0;
  if (job.status !== "completed" && job.status !== "cancelled") return 1;
  return 2;
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

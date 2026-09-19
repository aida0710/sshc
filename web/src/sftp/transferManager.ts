import { failureCode } from "../api/client";
import { sftpApi, type TransferJobList, type TransferKind, type TransferQueueMove } from "./api";
import { DownloadPlane } from "./downloadPlane";
import { TransferLedger, type ManagedTransferJob, type TransferNotice } from "./transferLedger";
import type { TransferManagerAPI, TransferPlaneContext } from "./transferPlane";
import { TransferSpeedometer } from "./transferProgress";
import { fingerprintFile } from "./uploadFingerprint";
import { UploadPlane } from "./uploadPlane";

export type { ManagedTransferJob, TransferNotice } from "./transferLedger";

const maxTransferJobs = 200;
const defaultLargeFileThreshold = 100 << 20;
// One connection unless the engine settings say otherwise; see the engine's
// DefaultLargeFileParallelism for why.
const defaultLargeFileParallelism = 1;
const defaultLargeFileChunkBytes = 32 << 20;
// How long a job waits after the engine refused to start it.
const retryDelayMs = 500;

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

function identifier(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function baseName(remotePath: string): string {
  const components = remotePath.split("/").filter(Boolean);
  return components[components.length - 1] ?? remotePath;
}

function missingServerTransfer(error: unknown): boolean {
  const code = failureCode(error) || (error instanceof Error ? error.message : "");
  return code === "sftp_transfer_not_found";
}

// The engine keeps the queue; the browser runs the uploads and downloads
// that need its files or its save dialog, and mirrors remote-to-remote jobs
// the engine runs by itself. This class schedules that work and exposes
// the queue to the UI; the byte pushing lives in the two planes.
export class SFTPTransferManager {
  private readonly ledger = new TransferLedger();
  private readonly uploads: UploadPlane;
  private readonly downloads: DownloadPlane;
  private readonly speed: TransferSpeedometer;
  private readonly controllers = new Map<string, AbortController>();
  private readonly inFlight = new Set<string>();
  private readonly uploadAdmissions = new Set<UploadAdmission>();
  private readonly controlOperations = new Map<string, Promise<void>>();
  private readonly retryAfter = new Map<string, number>();
  private active = 0;
  private maxConcurrent: number;
  private clearCompletedAfter = 0;
  private processingStopped = false;
  private largeFileThreshold = defaultLargeFileThreshold;
  private largeFileParallelism = defaultLargeFileParallelism;
  private largeFileChunkBytes = defaultLargeFileChunkBytes;

  constructor(
    private readonly api: TransferManagerAPI = sftpApi,
    concurrency = 2,
    private readonly now = () => Date.now(),
  ) {
    this.maxConcurrent = concurrency;
    this.speed = new TransferSpeedometer(now);
    const context: TransferPlaneContext = {
      ledger: this.ledger,
      arm: (id) => {
        const controller = new AbortController();
        this.controllers.set(id, controller);
        return controller;
      },
      progress: (id, transferredBytes, totalBytes) => this.recordProgress(id, transferredBytes, totalBytes),
      reconcile: () => this.reconcile(),
    };
    this.uploads = new UploadPlane(api, context);
    this.downloads = new DownloadPlane(api, context);
  }

  getSnapshot = (): readonly ManagedTransferJob[] => this.ledger.snapshot();
  getNoticeSnapshot = (): readonly TransferNotice[] => this.ledger.noticeSnapshot();
  getMaxConcurrent = (): number => this.maxConcurrent;
  getClearCompletedAfter = (): number => this.clearCompletedAfter;
  getProcessingStopped = (): boolean => this.processingStopped;
  getLargeFileThreshold = (): number => this.largeFileThreshold;
  getLargeFileParallelism = (): number => this.largeFileParallelism;
  getLargeFileChunkBytes = (): number => this.largeFileChunkBytes;
  hasUploadSource = (id: string): boolean => this.uploads.has(id);
  subscribe = (listener: () => void): (() => void) => this.ledger.subscribe(listener);
  subscribeNotices = (listener: () => void): (() => void) => this.ledger.subscribeNotices(listener);

  async reconcile(): Promise<void> {
    const requestedAt = this.ledger.generation();
    const listed = await this.api.listTransfers();
    const serverIDs = new Set(listed.jobs.map((job) => job.id));
    if (listed.jobs.length > maxTransferJobs || serverIDs.size !== listed.jobs.length) {
      throw new Error("sftp_transfer_limit");
    }
    if (!this.adoptQueue(listed, requestedAt)) {
      // Stale listing; the next poll lists again with the newer state.
      return;
    }
    await this.downloads.removeOrphans(new Set(listed.jobs
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
    this.adoptQueue(listed);
    this.kick();
  }

  // Only waiting jobs move, and the engine decides what "waiting" means at the
  // moment the request lands.
  async move(id: string, move: TransferQueueMove): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || job.status !== "queued") return;
    this.adoptQueue(await this.api.moveTransfer(id, move));
  }

  reserveUploads(selections: UploadSelection[]): UploadAdmission {
    const count = this.pendingUploadCount(selections);
    if (this.jobs.length + this.reservedUploads() + count > maxTransferJobs) throw new Error("sftp_transfer_limit");
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
    const newSelections = selections.filter((selection) => this.reattachable(selection) === undefined);
    const otherReserved = this.reservedUploads(admission);
    if (admission !== undefined && (!this.uploadAdmissions.has(admission) || newSelections.length > admission.count)) {
      throw new Error("sftp_transfer_limit");
    }
    if (this.jobs.length + otherReserved + newSelections.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
    admission?.release();
    const batchId = batch?.id ?? identifier("batch");
    const batchName = batch?.name ?? selections[0]?.localName ?? "upload";
    const batchKind = batch?.kind ?? (selections.length > 1 ? "folder" : "file");
    for (const selection of selections) {
      const existing = this.reattachable(selection);
      if (existing !== undefined) {
        await this.reattachUpload(existing, selection.file);
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
      this.uploads.attach(id, selection.file);
      this.ledger.commit([...this.jobs, job]);
    }
    this.kick();
    return batchId;
  }

  async addDownload(alias: string, remotePath: string, kind: TransferKind, totalBytes: number): Promise<string> {
    if (this.jobs.length + this.reservedUploads() >= maxTransferJobs) throw new Error("sftp_transfer_limit");
    const id = identifier("transfer");
    const name = baseName(remotePath);
    const job = await this.api.createTransfer({
      id, batchId: identifier("batch"),
      batchName: name, batchKind: kind, alias,
      direction: "download", kind, name, remotePath, totalBytes, lastModified: 0,
    });
    this.downloads.prepare(id);
    this.ledger.commit([...this.jobs, job]);
    this.kick();
    return id;
  }

  async addRemoteTransfers(selections: RemoteTransferSelection[], operation: "copy" | "move" | "delete" | "get" | "put"): Promise<string[]> {
    if (selections.length === 0) return [];
    if (this.jobs.length + this.reservedUploads() + selections.length > maxTransferJobs) throw new Error("sftp_transfer_limit");
    const batchId = identifier("remote_batch");
    const batchName = selections.length === 1 ? selections[0]!.name : `${selections.length} items`;
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
      this.ledger.commit([...this.jobs, job]);
      ids.push(id);
    }
    await this.reconcile();
    return ids;
  }

  async pause(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || !job.allowedActions.includes("pause")) return;
    this.controllers.get(id)?.abort();
    const operation = this.api.updateTransfer(id, "pause").then((updated) => { this.ledger.replaceServer(updated); });
    this.trackControl(id, operation);
    await operation;
  }

  async resume(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || !job.allowedActions.includes("resume") || !this.networkReady(job)) return;
    const restart = job.direction === "download" && job.kind === "folder";
    if (restart) {
      this.downloads.restart(id);
      this.ledger.replace(id, { transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1 });
    }
    const updated = await this.api.updateTransfer(id, "resume", { resetProgress: restart });
    this.ledger.replaceServer(updated);
    this.kick();
  }

  async retry(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || !job.allowedActions.includes("retry") || !this.networkReady(job)) return;
    const restart = job.direction === "download" && job.kind === "folder";
    if (restart) this.downloads.restart(id);
    const updated = await this.api.updateTransfer(id, "retry", { resetProgress: restart });
    this.ledger.replaceServer(updated);
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
      if (!this.ledger.find(job.id)?.allowedActions.includes("cancel")) continue;
      try {
        await this.cancel(job.id);
      } catch (error) {
        firstFailure ??= error;
      }
    }
    if (firstFailure !== undefined) throw firstFailure;
  }

  // Overwriting is a resume: the engine marks a job resumed from
  // needs_overwrite as allowed to replace the target. A browser upload also
  // needs its File still in memory, which networkReady checks.
  async overwrite(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || job.status !== "needs_overwrite") return;
    await this.resume(id);
  }

  async cancel(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || !job.allowedActions.includes("cancel")) return;
    this.controllers.get(id)?.abort();
    if (job.direction === "upload") {
      try {
        await this.api.cancelUpload(job.alias, id, job.remotePath);
      } catch (error) {
        if (missingServerTransfer(error)) {
          this.uploads.detach(id);
          await this.reconcile();
          return;
        }
        await this.reconcile().catch(() => undefined);
        throw error;
      }
      this.uploads.detach(id);
    } else {
      try {
        const updated = await this.api.updateTransfer(id, "cancel");
        this.ledger.replaceServer(updated);
      } catch (error) {
        if (!missingServerTransfer(error)) throw error;
      }
      await this.downloads.discard(id);
    }
    await this.reconcile();
  }

  async clearFinished(): Promise<void> {
    const removed = this.jobs.filter((job) => job.status === "completed" || job.status === "cancelled").map((job) => job.id);
    await this.api.clearFinishedTransfers();
    this.ledger.commit(this.jobs.filter((job) => job.status !== "completed" && job.status !== "cancelled"));
    for (const id of removed) {
      void this.downloads.discard(id);
    }
  }

  async remove(id: string): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || !job.allowedActions.includes("remove")) return;
    await this.api.removeTransfer(id);
    this.uploads.detach(id);
    await this.downloads.discard(id);
    this.ledger.commit(this.jobs.filter((candidate) => candidate.id !== id));
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
    this.ledger.dismissNotice(id);
  }

  private get jobs(): readonly ManagedTransferJob[] {
    return this.ledger.snapshot();
  }

  // Takes the engine's queue and settings as the new truth. The settings
  // always apply. The jobs apply unconditionally when the listing answered a
  // mutation of ours, and only if nothing newer was applied meanwhile when it
  // came from a poll (requestedAt is the generation seen before that poll).
  private adoptQueue(listed: TransferJobList, requestedAt?: number): boolean {
    this.maxConcurrent = listed.maxConcurrent;
    this.clearCompletedAfter = listed.clearCompletedAfterSeconds ?? 0;
    this.processingStopped = listed.processingStopped === true;
    this.largeFileThreshold = listed.largeFileThresholdBytes ?? defaultLargeFileThreshold;
    this.largeFileParallelism = listed.largeFileParallelism ?? defaultLargeFileParallelism;
    this.largeFileChunkBytes = listed.largeFileChunkBytes ?? defaultLargeFileChunkBytes;
    if (requestedAt === undefined) {
      this.ledger.replaceAllServer(listed.jobs);
      return true;
    }
    return this.ledger.adoptListing(listed.jobs, requestedAt);
  }

  // Whether the browser has what it takes to run the job right now: an
  // upload needs its File, which does not survive a reload.
  private networkReady(job: ManagedTransferJob): boolean {
    return job.direction === "remote" || job.direction === "download" || this.uploads.has(job.id);
  }

  // The listed job this selection is a second pick of: same file at the same
  // destination, waiting for its File after a reload.
  private reattachable(selection: UploadSelection): ManagedTransferJob | undefined {
    return this.jobs.find((job) => job.direction === "upload" && !this.uploads.has(job.id) &&
      ["queued", "running", "paused", "reattach", "failed", "needs_overwrite"].includes(job.status) &&
      job.alias === selection.alias && job.remotePath === selection.remotePath &&
      job.totalBytes === selection.file.size && job.lastModified === selection.file.lastModified);
  }

  private async reattachUpload(existing: ManagedTransferJob, file: File): Promise<void> {
    this.uploads.attach(existing.id, file);
    if (existing.status === "running") {
      await this.api.updateTransfer(existing.id, "pause");
      this.ledger.replaceServer(await this.api.updateTransfer(existing.id, "resume"));
    } else if (existing.status === "paused" || existing.status === "reattach") {
      this.ledger.replaceServer(await this.api.updateTransfer(existing.id, "resume"));
    } else if (existing.status === "failed") {
      this.ledger.replaceServer(await this.api.updateTransfer(existing.id, "retry"));
    }
  }

  private pendingUploadCount(selections: UploadSelection[]): number {
    return selections.filter((selection) => this.reattachable(selection) === undefined).length;
  }

  // Slots held by admissions other than the given one.
  private reservedUploads(except?: UploadAdmission): number {
    return [...this.uploadAdmissions].reduce((sum, admission) => sum + (admission === except ? 0 : admission.count), 0);
  }

  private kick(): void {
    // The engine refuses a start while the queue is stopped. Asking anyway
    // would just spend a request per waiting job on every tick.
    if (this.processingStopped) return;
    while (this.active < this.maxConcurrent) {
      const job = this.jobs.find((candidate) => candidate.status === "queued" &&
        candidate.direction !== "remote" &&
        !this.inFlight.has(candidate.id) && (this.retryAfter.get(candidate.id) ?? 0) <= this.now() &&
        this.networkReady(candidate));
      if (job === undefined) return;
      this.active += 1;
      this.inFlight.add(job.id);
      this.speed.start(job.id, job.transferredBytes);
      void this.run(job.id).finally(() => {
        this.active -= 1;
        this.inFlight.delete(job.id);
        this.controllers.delete(job.id);
        this.kick();
      });
    }
  }

  private async run(id: string): Promise<void> {
    let job = this.ledger.find(id);
    if (job === undefined) return;
    try {
      let sourceFingerprint = "";
      if (job.direction === "upload") {
        const file = this.uploads.get(id);
        if (file === undefined) return;
        const controller = new AbortController();
        this.controllers.set(id, controller);
        sourceFingerprint = await fingerprintFile(file, controller.signal, () => this.ledger.find(id)?.status === "queued");
        const current = this.ledger.find(id);
        if (current === undefined || current.status !== "queued") return;
        if (current.sourceFingerprint && current.sourceFingerprint !== sourceFingerprint) {
          throw new Error("sftp_upload_source_changed");
        }
        job = this.ledger.find(id)!;
      }
      if (!await this.prepareServer(job)) return;
      job = this.ledger.find(id);
      if (job === undefined || job.status !== "running") return;
      if (job.direction === "upload") await this.uploads.run(id, sourceFingerprint);
      else await this.downloads.run(id);
    } catch (error) {
      await this.settleFailure(id, error);
    }
  }

  // Decides what a thrown error means for the job: nothing, a short wait
  // for the engine, an overwrite question, or a failure the engine records.
  private async settleFailure(id: string, error: unknown): Promise<void> {
    const job = this.ledger.find(id);
    if (job === undefined || job.status === "paused" || job.status === "cancelled" ||
        (error instanceof DOMException && error.name === "AbortError")) return;
    const code = failureCode(error) || (error instanceof Error ? error.message : "sftp_failed");
    if (code === "sftp_transfer_limit" || code === "sftp_transfer_state") {
      this.retryAfter.set(id, this.now() + retryDelayMs);
      await this.reconcile().catch(() => undefined);
      globalThis.setTimeout(() => {
        this.retryAfter.delete(id);
        this.kick();
      }, retryDelayMs);
      return;
    }
    if (code === "sftp_exists" && job.direction === "upload" && !job.overwrite) {
      const updated = await this.api.updateTransfer(id, "needs_overwrite").catch(() => null);
      if (updated !== null) this.ledger.replaceServer(updated);
      return;
    }
    const failed = await this.api.updateTransfer(id, "fail", { problem: code }).catch(() => null);
    if (failed !== null) {
      this.ledger.replaceServer(failed);
      this.ledger.notify(failed);
    } else {
      await this.reconcile().catch(() => undefined);
    }
  }

  private async prepareServer(job: ManagedTransferJob): Promise<boolean> {
    await this.controlOperations.get(job.id);
    const current = this.ledger.find(job.id);
    if (current?.status !== "queued") return false;
    const started = await this.api.updateTransfer(job.id, "start");
    this.retryAfter.delete(job.id);
    this.ledger.replaceServer(started);
    return started.status === "running";
  }

  private recordProgress(id: string, transferredBytes: number, totalBytes: number): void {
    const job = this.ledger.find(id);
    if (job === undefined) return;
    this.ledger.replace(id, this.speed.measure(job, transferredBytes, totalBytes));
  }

  // A pause or cancel in flight must land before the next start is asked for.
  private trackControl(id: string, operation: Promise<void>): void {
    this.controlOperations.set(id, operation);
    const settled = () => {
      if (this.controlOperations.get(id) === operation) this.controlOperations.delete(id);
    };
    void operation.then(settled, settled);
  }
}

export const sftpTransferManager = new SFTPTransferManager();

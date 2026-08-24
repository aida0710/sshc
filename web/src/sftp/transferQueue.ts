import { failureCode } from "../api/client";
import { sftpApi } from "./api";

const chunkBytes = 1 << 20;
const persistenceKey = "sshc.sftp.transfer-queue.v1";

export type UploadJobStatus = "queued" | "uploading" | "paused" | "reattach" | "needs_overwrite" | "done" | "failed" | "cancelled";

export type UploadJob = {
  id: string;
  alias: string;
  remotePath: string;
  localName: string;
  size: number;
  lastModified: number;
  offset: number;
  status: UploadJobStatus;
  expectedRevision: string;
  overwrite: boolean;
  problem?: string | undefined;
};

export type UploadSelection = {
  alias: string;
  remotePath: string;
  localName: string;
  file: File;
};

type QueueAPI = Pick<typeof sftpApi, "startUpload" | "appendUpload" | "completeUpload" | "cancelUpload">;

function identifier(): string {
  return globalThis.crypto?.randomUUID?.() ?? `upload_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

function restorable(value: unknown): UploadJob[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((candidate) => {
    if (candidate === null || typeof candidate !== "object") return [];
    const job = candidate as Partial<UploadJob>;
    if (typeof job.id !== "string" || typeof job.alias !== "string" || typeof job.remotePath !== "string" ||
        typeof job.localName !== "string" || typeof job.size !== "number" || typeof job.lastModified !== "number" ||
        typeof job.offset !== "number" || typeof job.expectedRevision !== "string" || typeof job.overwrite !== "boolean") return [];
    const terminal = job.status === "done" || job.status === "cancelled";
    return [{ ...job, status: terminal ? job.status : "reattach" as const } as UploadJob];
  }).slice(-50);
}

export class SFTPTransferQueue {
  private jobs: UploadJob[];
  private readonly files = new Map<string, File>();
  private readonly controllers = new Map<string, AbortController>();
  private readonly listeners = new Set<() => void>();
  private active = 0;

  constructor(private readonly api: QueueAPI = sftpApi, private readonly concurrency = 2) {
    let restored: UploadJob[] = [];
    try {
      restored = restorable(JSON.parse(globalThis.localStorage?.getItem(persistenceKey) ?? "[]"));
    } catch {
      restored = [];
    }
    this.jobs = restored;
  }

  getSnapshot = (): readonly UploadJob[] => this.jobs;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  add(selections: UploadSelection[]): void {
    const additions: UploadJob[] = [];
    for (const selection of selections) {
      const existing = this.jobs.find((job) => job.alias === selection.alias && job.remotePath === selection.remotePath &&
        job.size === selection.file.size && job.lastModified === selection.file.lastModified && job.status === "reattach");
      if (existing !== undefined) {
        this.files.set(existing.id, selection.file);
        this.replace(existing.id, { status: "queued", problem: undefined });
        continue;
      }
      const job: UploadJob = {
        id: identifier(), alias: selection.alias, remotePath: selection.remotePath, localName: selection.localName,
        size: selection.file.size, lastModified: selection.file.lastModified, offset: 0, status: "queued",
        expectedRevision: "", overwrite: false,
      };
      this.files.set(job.id, selection.file);
      additions.push(job);
    }
    if (additions.length > 0) this.commit([...this.jobs, ...additions]);
    this.kick();
  }

  pause(id: string): void {
    const job = this.find(id);
    if (job === undefined || (job.status !== "uploading" && job.status !== "queued")) return;
    this.replace(id, { status: "paused" });
    this.controllers.get(id)?.abort();
  }

  resume(id: string): void {
    const job = this.find(id);
    if (job === undefined || (job.status !== "paused" && job.status !== "failed") || !this.files.has(id)) return;
    this.replace(id, { status: "queued", problem: undefined });
    this.kick();
  }

  overwrite(id: string): void {
    const job = this.find(id);
    if (job === undefined || job.status !== "needs_overwrite") return;
    this.replace(id, { overwrite: true, status: "queued", problem: undefined });
    this.kick();
  }

  async cancel(id: string): Promise<void> {
    const job = this.find(id);
    if (job === undefined || job.status === "done" || job.status === "cancelled") return;
    this.controllers.get(id)?.abort();
    this.replace(id, { status: "cancelled" });
    this.files.delete(id);
    try {
      await this.api.cancelUpload(job.alias, job.id, job.remotePath);
    } catch {
      // The UI state is cancelled even if an unreachable host delays part-file cleanup.
    }
  }

  clearFinished(): void {
    this.commit(this.jobs.filter((job) => job.status !== "done" && job.status !== "cancelled"));
  }

  private kick(): void {
    while (this.active < this.concurrency) {
      const job = this.jobs.find((candidate) => candidate.status === "queued" && this.files.has(candidate.id));
      if (job === undefined) return;
      this.active += 1;
      this.replace(job.id, { status: "uploading" });
      void this.run(job.id).finally(() => {
        this.active -= 1;
        this.controllers.delete(job.id);
        this.kick();
      });
    }
  }

  private async run(id: string): Promise<void> {
    const file = this.files.get(id);
    let job = this.find(id);
    if (file === undefined || job === undefined) return;
    try {
      const started = await this.api.startUpload(job.alias, job.id, job.remotePath, job.size, job.overwrite, job.expectedRevision);
      this.replace(id, { offset: started.offset, expectedRevision: started.expectedRevision });
      let offset = started.offset;
      while (offset < file.size) {
        job = this.find(id);
        if (job === undefined || job.status !== "uploading") return;
        const controller = new AbortController();
        this.controllers.set(id, controller);
        const end = Math.min(offset + chunkBytes, file.size);
        const appended = await this.retry(() => this.api.appendUpload(job!.alias, id, job!.remotePath, offset, file.size, file.slice(offset, end), controller.signal));
        offset = appended.offset;
        this.replace(id, { offset });
      }
      job = this.find(id);
      if (job === undefined || job.status !== "uploading") return;
      await this.api.completeUpload(job.alias, id, job.remotePath, job.size, job.expectedRevision);
      this.files.delete(id);
      this.replace(id, { offset: job.size, status: "done" });
    } catch (error) {
      job = this.find(id);
      if (job === undefined || job.status === "paused" || job.status === "cancelled") return;
      const code = failureCode(error);
      if (code === "sftp_exists" && !job.overwrite) {
        this.replace(id, { status: "needs_overwrite" });
        return;
      }
      this.replace(id, { status: "failed", problem: code || (error instanceof Error ? error.message : "upload_failed") });
    }
  }

  private async retry<T>(operation: () => Promise<T>): Promise<T> {
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

  private find(id: string): UploadJob | undefined {
    return this.jobs.find((job) => job.id === id);
  }

  private replace(id: string, changes: Partial<UploadJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  private commit(jobs: UploadJob[]): void {
    this.jobs = jobs;
    try {
      globalThis.localStorage?.setItem(persistenceKey, JSON.stringify(jobs));
    } catch {
      // A disabled or full storage must not stop an in-memory transfer.
    }
    for (const listener of this.listeners) listener();
  }
}

export const sftpTransferQueue = new SFTPTransferQueue();

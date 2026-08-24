import { sftpApi, type TransferOptions } from "./api";

export type DownloadJob = {
  id: string;
  alias: string;
  remotePath: string;
  directory: boolean;
  bytes: number;
  total: number | null;
  status: "downloading" | "done" | "failed" | "cancelled";
  problem?: string | undefined;
};

type DownloadAPI = (alias: string, remotePath: string, directory: boolean, options: TransferOptions) => Promise<number>;

function identifier(): string {
  return globalThis.crypto?.randomUUID?.() ?? `download_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

export class SFTPDownloadQueue {
  private jobs: DownloadJob[] = [];
  private readonly listeners = new Set<() => void>();
  private readonly controllers = new Map<string, AbortController>();

  constructor(private readonly downloadAPI: DownloadAPI = sftpApi.download) {}

  getSnapshot = (): readonly DownloadJob[] => this.jobs;
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  add(alias: string, remotePath: string, directory: boolean, total: number | null): void {
    const job: DownloadJob = { id: identifier(), alias, remotePath, directory, bytes: 0, total, status: "downloading" };
    const controller = new AbortController();
    this.controllers.set(job.id, controller);
    this.commit([...this.jobs, job]);
    void this.downloadAPI(alias, remotePath, directory, {
      signal: controller.signal,
      onProgress: (bytes, reportedTotal) => this.replace(job.id, { bytes, total: reportedTotal ?? total }),
    }).then((bytes) => this.replace(job.id, { bytes, status: "done" })).catch((error: unknown) => {
      const current = this.jobs.find((candidate) => candidate.id === job.id);
      if (current?.status === "cancelled") return;
      this.replace(job.id, { status: "failed", problem: error instanceof Error ? error.message : "download_failed" });
    }).finally(() => this.controllers.delete(job.id));
  }

  cancel(id: string): void {
    const job = this.jobs.find((candidate) => candidate.id === id);
    if (job === undefined || job.status !== "downloading") return;
    this.replace(id, { status: "cancelled" });
    this.controllers.get(id)?.abort();
  }

  clearFinished(): void {
    this.commit(this.jobs.filter((job) => job.status === "downloading"));
  }

  private replace(id: string, changes: Partial<DownloadJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  private commit(jobs: DownloadJob[]): void {
    this.jobs = jobs;
    for (const listener of this.listeners) listener();
  }
}

export const sftpDownloadQueue = new SFTPDownloadQueue();

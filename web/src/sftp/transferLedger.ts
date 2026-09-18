import { notifyAndroidTransfer } from "../android/native";
import type { TransferJob } from "./api";

export type ManagedTransferJob = TransferJob;

export type TransferNotice = {
  id: string;
  jobId: string;
  status: "completed" | "failed";
  name: string;
  direction: "upload" | "download" | "remote";
  problem: string;
};

// Older notices fall off once this many precede a new one.
const keptNotices = 7;

// The browser's copy of the engine's transfer queue, and the completion
// notices raised from it. Every change goes through commit, so a store
// subscribed to the snapshot sees one new array per change and nothing else.
export class TransferLedger {
  private jobs: ManagedTransferJob[] = [];
  private notices: TransferNotice[] = [];
  private readonly listeners = new Set<() => void>();
  private readonly noticeListeners = new Set<() => void>();

  snapshot(): readonly ManagedTransferJob[] {
    return this.jobs;
  }

  noticeSnapshot(): readonly TransferNotice[] {
    return this.notices;
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  subscribeNotices(listener: () => void): () => void {
    this.noticeListeners.add(listener);
    return () => this.noticeListeners.delete(listener);
  }

  find(id: string): ManagedTransferJob | undefined {
    return this.jobs.find((job) => job.id === id);
  }

  replace(id: string, changes: Partial<ManagedTransferJob>): void {
    this.commit(this.jobs.map((job) => job.id === id ? { ...job, ...changes } : job));
  }

  // Takes the engine's view of one job, appending it when it is new here.
  replaceServer(updated: TransferJob): void {
    this.commit(this.jobs.some((job) => job.id === updated.id)
      ? this.jobs.map((job) => job.id === updated.id ? updated : job)
      : [...this.jobs, updated]);
  }

  commit(jobs: ManagedTransferJob[]): void {
    this.jobs = jobs;
    for (const listener of this.listeners) listener();
  }

  // Raises a notice for a job that just finished; repeats of the same
  // attempt are ignored so that a reconcile cannot announce it twice.
  notify(job: ManagedTransferJob): void {
    if (job.status !== "completed" && job.status !== "failed") return;
    const notice: TransferNotice = {
      id: `${job.id}:${job.status}:${job.attempt}`, jobId: job.id, status: job.status,
      name: job.name, direction: job.direction, problem: job.problem,
    };
    if (this.notices.some((current) => current.id === notice.id)) return;
    this.notices = [...this.notices.slice(-keptNotices), notice];
    notifyAndroidTransfer(job.status);
    for (const listener of this.noticeListeners) listener();
  }

  dismissNotice(id: string): void {
    this.notices = this.notices.filter((notice) => notice.id !== id);
    for (const listener of this.noticeListeners) listener();
  }
}

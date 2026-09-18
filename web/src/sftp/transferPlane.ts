import type {
  CreateTransferJob,
  ResumableUpload,
  StreamDownloadOptions,
  TransferJob,
  TransferJobAction,
  TransferJobList,
  TransferQueueMove,
  TransferSettings,
} from "./api";
import type { TransferLedger } from "./transferLedger";

export type TransferManagerAPI = {
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

// What a transfer plane gets from the manager that scheduled it.
export type TransferPlaneContext = {
  ledger: TransferLedger;
  // A fresh controller for the job's next request; pause and cancel abort it.
  arm(id: string): AbortController;
  // Records bytes moved so far; a negative total leaves the total as is.
  progress(id: string, transferredBytes: number, totalBytes: number): void;
  reconcile(): Promise<void>;
};

// Three tries with a short back-off, for the one request that hit a hiccup.
// An abort is the user's doing and is not retried.
export async function retryOperation<T>(operation: () => Promise<T>): Promise<T> {
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

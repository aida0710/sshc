import type {
  BrowserSave,
  CreateTransferJob,
  DownloadStream,
  DownloadTarget,
  ResumableUpload,
  StreamDownloadOptions,
  TransferJob,
  TransferJobAction,
  TransferJobList,
  TransferQueueMove,
  TransferSettings,
  UploadChunk,
  UploadCompletion,
  UploadStart,
  UploadTarget,
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
  verifyDownload(target: DownloadTarget, revision: string): Promise<void>;
  startUpload(upload: UploadStart): Promise<ResumableUpload>;
  appendUpload(chunk: UploadChunk): Promise<ResumableUpload>;
  completeUpload(completion: UploadCompletion): Promise<void>;
  cancelUpload(target: UploadTarget): Promise<void>;
  streamDownload(download: DownloadStream, options: StreamDownloadOptions): Promise<{ bytes: number; total: number | null }>;
  saveDownload(remotePath: string, directory: boolean, chunks: BlobPart[]): Promise<BrowserSave | null>;
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

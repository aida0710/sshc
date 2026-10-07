import { sftpProblemCode } from "./sftpProblemText";

// Publication and local save are commit points: a missing acknowledgement
// must be reconciled, never treated as another data-transfer attempt.
export class TransferPublicationUncertain extends Error {
  constructor() { super("sftp_reconciliation_required"); }
}

export function retryableTransferFailure(error: unknown): boolean {
  if (error instanceof TransferPublicationUncertain) return false;
  return sftpProblemCode(error) === "sftp_connection_lost" || error instanceof TypeError;
}

// Pause, cancel and changing the queue settings abort the same wait as I/O.
export function waitForTransferReconnect(signal: AbortSignal, delayMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new DOMException("Aborted", "AbortError")); return; }
    const aborted = () => {
      globalThis.clearTimeout(timer);
      reject(new DOMException("Aborted", "AbortError"));
    };
    const timer = globalThis.setTimeout(() => {
      signal.removeEventListener("abort", aborted);
      resolve();
    }, Math.max(0, delayMs));
    signal.addEventListener("abort", aborted, { once: true });
  });
}

import type { ManagedTransferJob } from "./transferManager";
import { localHostAlias } from "./localHost";

export type TransferEndpoint = { alias: string; path: string } | { browser: "source" | "destination"; path: string };

export function getTransferJobRoute(job: ManagedTransferJob): { source: TransferEndpoint; destination: TransferEndpoint | null } {
  if (job.operation === "delete") return {
    source: { alias: job.sourceAlias || job.alias, path: job.sourcePath || job.remotePath }, destination: null,
  };
  // get/put persist the SSH alias in both fields; their local path belongs to
  // the engine, whereas upload/download involve the browser's file picker.
  if (job.operation === "get") return {
    source: { alias: job.sourceAlias || job.alias, path: job.sourcePath },
    destination: { alias: localHostAlias, path: job.remotePath },
  };
  if (job.operation === "put") return {
    source: { alias: localHostAlias, path: job.sourcePath },
    destination: { alias: job.alias, path: job.remotePath },
  };
  if (job.direction === "upload") return {
    source: { browser: "source", path: job.name }, destination: { alias: job.alias, path: job.remotePath },
  };
  if (job.direction === "download") return {
    source: { alias: job.alias, path: job.remotePath }, destination: { browser: "destination", path: "" },
  };
  return {
    source: { alias: job.sourceAlias || job.alias, path: job.sourcePath },
    destination: { alias: job.alias, path: job.remotePath },
  };
}

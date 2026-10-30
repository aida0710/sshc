// What the engine's transfer queue uses for a setting no preset chose (internal/sftp/jobs_settings.go).
export const defaultTransferConcurrency = 2;
export const defaultLargeFileThreshold = 100 << 20;
// One connection unless a preset says otherwise; see the engine's DefaultLargeFileParallelism for why.
export const defaultLargeFileParallelism = 1;
export const defaultLargeFileChunkBytes = 32 << 20;

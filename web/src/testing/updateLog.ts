import type { UpdateLog } from "../api/update";

// updateLog is an UpdateLog of job "job-one" whose output from `from` on is
// all in this one answer. A test names only the fields it is about.
export function updateLog(output: string, from = 0, overrides: Partial<UpdateLog> = {}): UpdateLog {
  const end = from + output.length;
  return {
    jobId: "job-one",
    reachedCapacity: false,
    cursor: { requested: from, start: from, next: end, end, truncated: false },
    output,
    ...overrides,
  };
}

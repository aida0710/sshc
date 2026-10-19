import { useCallback, useEffect, useRef, useState } from "react";
import type { UpdateStatus } from "../api/update";
import { isUpdateActive } from "../shell/useUpdateStatus";
import { updateJobActivity, updateJobActivityKey, type UpdateActivity } from "./updateActivity";

// The log explains the checks and the update made while the page is open; older
// lines are dropped so that repeated checks do not push the release notes away.
const maxActivityEntries = 20;

export type UpdateActivityEntry = UpdateActivity & { id: number; at: Date };

// useUpdateActivity keeps the About page's log of checks and update progress. It
// is not stored: reopening the page starts an empty log.
export function useUpdateActivity(status: UpdateStatus | null) {
  const [entries, setEntries] = useState<UpdateActivityEntry[]>([]);
  const nextId = useRef(0);
  // undefined until the first status arrives; null while the engine reports no job.
  const reportedJob = useRef<string | null | undefined>(undefined);

  const record = useCallback((activity: UpdateActivity) => {
    const entry = { ...activity, id: nextId.current++, at: new Date() };
    setEntries((current) => [...current, entry].slice(-maxActivityEntries));
  }, []);

  const job = status?.job;
  const jobKey = job === undefined ? null : updateJobActivityKey(job);
  useEffect(() => {
    if (status === null) return;
    const previous = reportedJob.current;
    reportedJob.current = jobKey;
    if (job === undefined || jobKey === previous) return;
    // A job that had already finished when the page opened is not part of this page's log.
    if (previous === undefined && !isUpdateActive(job)) return;
    record(updateJobActivity(job));
  }, [status, job, jobKey, record]);

  return { entries, record };
}

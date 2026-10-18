import { useCallback, useEffect, useRef, useState } from "react";
import type { UpdateApi, UpdateJob, UpdateStatus } from "../api/update";
import { forgetUpdateResultToCheck, readUpdateResultToCheck } from "./updateResultToCheck";

// Active jobs update quickly enough to show installer/restart progress.
const updatePollIntervalMs = 2000;

export function isUpdateActive(job: UpdateJob | undefined): boolean {
  return job !== undefined && ["accepted", "installing", "restarting"].includes(job.state);
}

// The engine keeps reporting the last job until the next one starts. A success is
// news only to the page that watched the job run or was reloaded to check it; to
// every later page it would stay under the version, even after sshc was updated
// another way. Failures still need action, so they are always shown.
export function withoutUnwatchedSuccess(status: UpdateStatus, watchedJobIds: ReadonlySet<string>): UpdateStatus {
  if (status.job?.state !== "succeeded" || watchedJobIds.has(status.job.id)) return status;
  const { job: _unwatched, ...withoutJob } = status;
  return withoutJob;
}

export function useUpdateStatus(api: UpdateApi, enabled = true) {
  const [loadedStatus, setLoadedStatus] = useState<UpdateStatus | null>(null);
  const watchedJobIds = useRef<Set<string> | null>(null);
  if (watchedJobIds.current === null) {
    const handedOver = readUpdateResultToCheck();
    watchedJobIds.current = new Set(handedOver === null ? [] : [handedOver]);
  }
  const activeJob = isUpdateActive(loadedStatus?.job);

  const setStatus = useCallback((status: UpdateStatus) => {
    if (status.job !== undefined && isUpdateActive(status.job)) watchedJobIds.current?.add(status.job.id);
    setLoadedStatus(status);
  }, []);

  // Read during the first render and cleared here, so a repeated first render still sees it.
  useEffect(() => { forgetUpdateResultToCheck(); }, []);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function load() {
      try {
        const loaded = await api.updateStatus();
        if (active) setStatus(loaded);
      } catch {
        // Restart temporarily removes the listener. Retain the durable job and
        // let the existing session-recovery screen handle a replaced session.
      }
      if (active && activeJob) {
        timer = setTimeout(() => { void load(); }, updatePollIntervalMs);
      }
    }

    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [api, activeJob, enabled, setStatus]);

  const status = loadedStatus === null ? null : withoutUnwatchedSuccess(loadedStatus, watchedJobIds.current);
  return { status, setStatus };
}

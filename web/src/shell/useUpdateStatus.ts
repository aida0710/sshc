import { useEffect, useState } from "react";
import type { UpdateApi, UpdateJob, UpdateStatus } from "../api/update";

// Active jobs update quickly enough to show installer/restart progress.
const updatePollIntervalMs = 2000;

export function isUpdateActive(job: UpdateJob | undefined): boolean {
  return job !== undefined && ["accepted", "installing", "restarting"].includes(job.state);
}

export function useUpdateStatus(api: UpdateApi, enabled = true) {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const activeJob = isUpdateActive(status?.job);

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
  }, [api, activeJob, enabled]);

  return { status, setStatus };
}

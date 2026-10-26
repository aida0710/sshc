import { useCallback, useEffect, useState } from "react";
import type { UpdateApi, UpdateJob, UpdateStatus } from "../api/update";

// Active jobs update quickly enough to show installer/restart progress.
const updatePollIntervalMs = 2000;
// The engine checks GitHub every six hours; open pages reread its retained result hourly.
const releasePollIntervalMs = 60 * 60 * 1000;

// A manual check or installation started on About must also reach the sidebar's result dialog.
const statusListeners = new Map<UpdateApi, Set<(status: UpdateStatus) => void>>();

export function isUpdateActive(job: UpdateJob | undefined): boolean {
  return job !== undefined && ["accepted", "installing", "restarting"].includes(job.state);
}

// availableRelease is the newer version that has been published, if there is one.
export function availableRelease(status: UpdateStatus | null): string | undefined {
  return status?.available === true && status.latest !== undefined ? status.latest : undefined;
}

export function useUpdateStatus(api: UpdateApi, enabled = true) {
  const [status, receiveStatus] = useState<UpdateStatus | null>(null);
  const setStatus = useCallback((next: UpdateStatus) => {
    for (const listener of statusListeners.get(api) ?? []) listener(next);
  }, [api]);

  useEffect(() => {
    const listeners = statusListeners.get(api) ?? new Set();
    listeners.add(receiveStatus);
    statusListeners.set(api, listeners);
    return () => {
      listeners.delete(receiveStatus);
      if (listeners.size === 0) statusListeners.delete(api);
    };
  }, [api]);
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
      if (active) {
        timer = setTimeout(() => { void load(); }, activeJob ? updatePollIntervalMs : releasePollIntervalMs);
      }
    }

    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [api, activeJob, enabled, setStatus]);

  return { status, setStatus };
}

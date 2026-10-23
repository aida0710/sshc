import { useEffect, useRef, useState } from "react";
import type { UpdateApi, UpdateJob, UpdateLog } from "../api/update";
import { isUpdateActive } from "../shell/useUpdateStatus";
import { useCommandOutput } from "../ui/useCommandOutput";
import { usePolling } from "../ui/usePolling";

// updateOutputPollMs is how often the installer's output is read while an
// update runs. Installers write a line every few seconds, so once a second
// shows each line as it comes without asking more often.
const updateOutputPollMs = 1000;

// maxUpdateOutput keeps all of a job's output on screen: it is the 256 KiB the
// engine records for one job (LogCapacity).
const maxUpdateOutput = 256 * 1024;

export type UpdateOutputState = {
  output: string;
  // reachedCapacity says the engine stopped recording at its limit, so later
  // output is missing from what is shown.
  reachedCapacity: boolean;
};

// useUpdateOutput follows what the installer and the engine restart write for
// the update job the engine reports. It keeps reading through the restart,
// when requests fail until the new engine answers, and resumes from where it
// stopped. The output of a job that has ended is read once to its end, so a
// page reloaded after the restart still shows how the update went.
export function useUpdateOutput(api: UpdateApi, job: UpdateJob | undefined): UpdateOutputState {
  const { output, nextCursor, receive, restart } = useCommandOutput(maxUpdateOutput);
  const [reachedCapacity, setReachedCapacity] = useState(false);
  // readToEnd turns true once the whole output of an ended job has been read.
  const [readToEnd, setReadToEnd] = useState(false);
  const jobId = job?.id;
  const active = isUpdateActive(job);
  // followedJob is the job whose output is shown; a read answered after the
  // engine reported another job is not added to it.
  const followedJob = useRef(jobId);

  useEffect(() => {
    followedJob.current = jobId;
    restart();
    setReachedCapacity(false);
    setReadToEnd(false);
  }, [jobId, restart]);

  usePolling(async (isCurrent) => {
    const followed = followedJob.current;
    let log: UpdateLog;
    try {
      log = await api.updateLog(nextCursor());
    } catch {
      // While the job runs, a failed read is the engine restarting and is
      // tried again; once it has ended, there is nothing more to wait for.
      if (isCurrent() && !active) setReadToEnd(true);
      return;
    }
    if (!isCurrent() || followedJob.current !== followed) return;
    if (log.jobId !== followed) {
      // The log still belongs to an earlier job.
      if (!active) setReadToEnd(true);
      return;
    }
    receive(log);
    setReachedCapacity(log.reachedCapacity);
    if (!active && log.cursor.next >= log.cursor.end) setReadToEnd(true);
  }, { intervalMs: updateOutputPollMs, enabled: jobId !== undefined && (active || !readToEnd), immediately: true });

  return { output, reachedCapacity };
}

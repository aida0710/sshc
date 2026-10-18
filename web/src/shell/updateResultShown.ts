import type { UpdateJob, UpdateStatus } from "../api/update";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { isUpdateActive } from "./useUpdateStatus";

// The engine keeps reporting the last update job until the next one starts. Its
// progress and result open in a dialog: progress while it runs, and the result
// once per job in this browser.

export function readShownUpdateResult(): string | null {
  return readStoredValue(localStorageKeys.updateShownResult);
}

export function rememberShownUpdateResult(jobId: string) {
  writeStoredValue(localStorageKeys.updateShownResult, jobId);
}

// updateJobToShow returns the job whose dialog should open. shownResultId is the job
// whose result this browser had shown when the page loaded.
export function updateJobToShow(status: UpdateStatus, shownResultId: string | null): UpdateJob | undefined {
  const job = status.job;
  if (job === undefined || isUpdateActive(job)) return job;
  if (job.id === shownResultId) return undefined;
  // A success for another version means sshc was updated another way since; it is no news.
  if (job.state === "succeeded" && (job.installedVersion ?? job.target) !== status.current) return undefined;
  return job;
}

// A closed progress dialog stays closed until the job finishes; the result opens anew.
export function updateDialogKey(job: UpdateJob): string {
  return `${job.id}:${isUpdateActive(job) ? "progress" : "result"}`;
}

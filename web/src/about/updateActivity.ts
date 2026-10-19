import type { UpdateJob, UpdateStatus } from "../api/update";
import type { MessageKey } from "../i18n/messages/en";
import { updateJobMessage } from "../shell/updateMessage";

// One step of a check or an update, as the About page saw it. The message is kept
// as a key so that the line follows a change of the display language.
export type UpdateActivity = {
  message: MessageKey;
  values?: Record<string, string>;
  tone: "progress" | "danger";
};

export const checkStartedActivity: UpdateActivity = { message: "about.activityChecking", tone: "progress" };

// The engine answered the check. A failed check keeps the previous release, so the
// failure is reported here and its reason stays in the notice above the buttons.
export function checkResultActivity(status: UpdateStatus): UpdateActivity {
  if (status.checkError !== undefined && status.checkError !== "") {
    return { message: "about.activityCheckFailed", tone: "danger" };
  }
  if (status.latest === undefined) {
    return { message: "about.activityNoRelease", tone: "progress" };
  }
  return { message: "about.activityLatest", values: { version: status.latest }, tone: "progress" };
}

// The request to the engine itself failed, so GitHub was never asked.
export const checkRequestFailedActivity: UpdateActivity = { message: "about.activityRequestFailed", tone: "danger" };

// Each state of an update job is reported once; the key changes when the state does.
export function updateJobActivityKey(job: UpdateJob): string {
  return `${job.id}:${job.state}`;
}

export function updateJobActivity(job: UpdateJob): UpdateActivity {
  const failed = job.state === "failed" || job.state === "restart_required";
  return {
    message: updateJobMessage(job),
    values: { version: job.installedVersion ?? job.target },
    tone: failed ? "danger" : "progress",
  };
}

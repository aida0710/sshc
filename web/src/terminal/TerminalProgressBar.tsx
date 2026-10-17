import { useTranslate } from "../i18n/context";
import type { TerminalProgress } from "./osc9Progress";

const barColors = {
  normal: "bg-accent",
  error: "bg-danger",
  paused: "bg-notice-ink",
} as const;

// Shared by pane headers and session rows so OSC 9;4 reports have the same
// appearance and accessible name wherever the session is shown.
export function TerminalProgressBar({ progress }: { progress: TerminalProgress }) {
  const t = useTranslate();
  if (progress.state === "none") return null;
  const indeterminate = progress.state === "indeterminate";
  return (
    <div
      role="progressbar"
      aria-label={t(progress.state === "error"
        ? "terminal.programProgressError"
        : progress.state === "paused" ? "terminal.programProgressPaused" : "terminal.programProgress")}
      aria-valuemin={0}
      aria-valuemax={100}
      {...(indeterminate ? {} : { "aria-valuenow": progress.percent })}
      className="pointer-events-none absolute inset-x-0 bottom-0 h-0.5 overflow-hidden"
    >
      {indeterminate ? (
        <div className="h-full w-1/3 animate-[sshc-progress-indeterminate_1.4s_ease-in-out_infinite] bg-accent" />
      ) : (
        <div className={`h-full ${barColors[progress.state]}`} style={{ width: `${progress.percent}%` }} />
      )}
    </div>
  );
}

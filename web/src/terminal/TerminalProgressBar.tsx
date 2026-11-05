import { useTranslate } from "../i18n/context";
import type { TerminalProgress } from "./osc9Progress";
import { programProgressName } from "./programProgress";

const barColors = {
  normal: "bg-accent",
  error: "bg-danger",
  paused: "bg-notice-ink",
} as const;

// The track shows the whole length, so that 40% reads as a share of it and not as a short underline under the title.
const trackColors = {
  normal: "bg-accent/20",
  indeterminate: "bg-accent/20",
  error: "bg-danger/20",
  paused: "bg-notice-ink/20",
} as const;

// TerminalProgressBar is the OSC 9;4 progress along the bottom of a pane's header. The session's row in the list
// shows the same progress as a ring in place of its dot (SessionMarker).
export function TerminalProgressBar({ progress }: { progress: TerminalProgress }) {
  const t = useTranslate();
  if (progress.state === "none") return null;
  const indeterminate = progress.state === "indeterminate";
  return (
    <div
      role="progressbar"
      aria-label={programProgressName(t, progress)}
      aria-valuemin={0}
      aria-valuemax={100}
      {...(indeterminate ? {} : { "aria-valuenow": progress.percent })}
      className={`pointer-events-none absolute inset-x-0 bottom-0 h-[3px] overflow-hidden ${trackColors[progress.state]}`}
    >
      {indeterminate ? (
        <div className="h-full w-1/3 animate-[sshc-progress-indeterminate_1.4s_ease-in-out_infinite] bg-accent" />
      ) : (
        <div className={`h-full ${barColors[progress.state]}`} style={{ width: `${progress.percent}%` }} />
      )}
    </div>
  );
}

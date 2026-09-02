import type { ReactNode } from "react";

export type PanelStateTone = "loading" | "empty" | "failed";

const toneClasses: Record<PanelStateTone, string> = {
  loading: "text-ink-muted",
  empty: "text-ink-muted",
  failed: "text-danger",
};

// One shape for the three answers a panel can give instead of content: it is
// still coming, there is none, or it could not be fetched. Panels differ in
// what they load, not in how they say these three things.
export function PanelState({
  tone,
  title,
  detail,
  action,
  icon,
  className = "",
}: {
  tone: PanelStateTone;
  title: string;
  detail?: string;
  action?: ReactNode;
  icon?: ReactNode;
  className?: string;
}) {
  return (
    <div
      role={tone === "failed" ? "alert" : "status"}
      aria-busy={tone === "loading"}
      className={`flex min-h-32 flex-col items-center justify-center gap-2 px-4 py-8 text-center ${className}`}
    >
      {icon}
      <p className={`text-sm ${toneClasses[tone]}`}>{title}</p>
      {detail === undefined ? null : <p className="max-w-prose text-xs text-ink-faint">{detail}</p>}
      {action}
    </div>
  );
}

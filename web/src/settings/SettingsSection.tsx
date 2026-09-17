import type { ReactNode } from "react";
import { Icon, type IconName } from "../ui/icons";

// One block of the settings page: a heading in the margin on the full page,
// none when the block is shown on its own.
export function SettingsSection({
  id,
  label,
  icon,
  showHeading = true,
  children,
}: {
  id: string;
  label: string;
  icon: IconName;
  showHeading?: boolean;
  children: ReactNode;
}) {
  return (
    <section id={id} aria-label={label} className="scroll-mt-6 border-b border-line last:border-b-0">
      <div className={showHeading
        ? "grid gap-5 px-4 py-6 sm:px-6 lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-8 lg:py-8"
        : "px-4 py-6 sm:px-6 lg:px-8 lg:py-8"}
      >
        {showHeading ? (
          <header className="flex items-center gap-3 self-start lg:sticky lg:top-6">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-select-fill text-accent">
              <Icon name={icon} />
            </span>
            <h3 className="text-sm font-semibold text-ink">{label}</h3>
          </header>
        ) : null}
        <div className="min-w-0">{children}</div>
      </div>
    </section>
  );
}

// The row under a form: what happened on the left, what to press on the right.
export function ActionArea({ children, status }: { children: ReactNode; status?: ReactNode }) {
  return (
    <div className="mt-6 flex min-h-12 flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
      <div className="min-w-0 flex-1">{status}</div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

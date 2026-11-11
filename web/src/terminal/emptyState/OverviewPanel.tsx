import type { ReactNode } from "react";
import { Icon, type IconName } from "../../ui/icons";

// OverviewPanel is one of the translucent cards under the tiles of the empty terminal (the activity and the
// recent connections). The art behind shows through, blurred, so the card reads over it.
export function OverviewPanel({
  icon,
  title,
  action,
  footer,
  children,
}: {
  icon: IconName;
  title: string;
  action?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="flex min-w-0 flex-col rounded-card border border-line bg-[color-mix(in_srgb,var(--ui-card)_84%,transparent)] px-4 py-3.5 backdrop-blur-[10px]" aria-label={title}>
      <div className="mb-2.5 flex items-center gap-2 text-ui font-semibold text-ink">
        <Icon name={icon} className="size-[15px] shrink-0 text-ink-muted" />
        <h3>{title}</h3>
        {action === undefined ? null : <div className="ml-auto">{action}</div>}
      </div>
      {children}
      {footer === undefined ? null : <div className="mt-auto flex justify-between gap-3 pt-2.5 text-xs text-ink-muted">{footer}</div>}
    </section>
  );
}

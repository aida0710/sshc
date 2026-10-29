import type { ReactNode } from "react";
import { Icon, type IconName } from "../ui/icons";

// A file preview's controls are signs alone, with their names for screen readers and as the tooltip. On a
// narrow screen they keep the 44px a finger needs.
export const previewIconButtonClassName = "flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40 disabled:hover:bg-transparent md:size-7 md:min-h-0 md:min-w-0";

export function PreviewIconButton({ icon, label, disabled = false, onClick }: { icon: IconName; label: string; disabled?: boolean; onClick: () => void }): ReactNode {
  return (
    <button type="button" className={previewIconButtonClassName} aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      <Icon name={icon} className="size-4" />
    </button>
  );
}

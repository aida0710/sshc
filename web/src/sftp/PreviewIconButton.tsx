import type { ReactNode } from "react";
import { IconButton } from "../ui/IconButton";
import { Icon, type IconName } from "../ui/icons";

// A file preview's controls are signs alone, with their names for screen readers and as the tooltip.

export function PreviewIconButton({ icon, label, disabled = false, onClick }: { icon: IconName; label: string; disabled?: boolean; onClick: () => void }): ReactNode {
  return (
    <IconButton label={label} size="toolbar" disabled={disabled} onClick={onClick} className="disabled:hover:bg-transparent">
      <Icon name={icon} className="size-4" />
    </IconButton>
  );
}

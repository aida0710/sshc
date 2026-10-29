import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Icon, type IconName } from "../ui/icons";

// The controls are signs alone, with their names for screen readers and as the tooltip. On a narrow screen
// the path takes the first row on its own and the controls move to the second; there they keep the 44px a
// finger needs.
const iconButtonClassName = "flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40 disabled:hover:bg-transparent md:size-7 md:min-h-0 md:min-w-0";

function IconButton({ icon, label, disabled = false, onClick }: { icon: IconName; label: string; disabled?: boolean; onClick: () => void }): ReactNode {
  return (
    <button type="button" className={iconButtonClassName} aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      <Icon name={icon} className="size-4" />
    </button>
  );
}

// FilePreviewHeader names the shown file and holds the preview's controls:
// stepping through the other printed files, downloading, the "…" of what
// else the file offers (menu), and closing.
export function FilePreviewHeader({
  headingId,
  path,
  position,
  count,
  onStep,
  onDownload,
  menu,
  onClose,
}: {
  headingId: string;
  path: string;
  position: number;
  count: number;
  onStep: (position: number) => void;
  onDownload: () => void;
  menu?: { label: string; items: ActionMenuItem[] };
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-line bg-toolbar px-3 py-2">
      {/* A long path loses its start, not the file name: the right-to-left box
          puts the ellipsis on the left, and the bdi keeps the path itself in
          reading order. */}
      <h2 id={headingId} dir="rtl" className="min-w-0 basis-full truncate text-left font-mono text-sm font-medium md:basis-0 md:grow" title={path}>
        <bdi dir="ltr">{path}</bdi>
      </h2>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        {count > 1 ? (
          <>
            <IconButton icon="chevronLeft" label={t("sftp.previewPrevious")} disabled={position === 0} onClick={() => onStep(position - 1)} />
            <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-ink-muted">
              {t("sftp.previewPosition", { position: String(position + 1), count: String(count) })}
            </span>
            <IconButton icon="chevronRight" label={t("sftp.previewNext")} disabled={position === count - 1} onClick={() => onStep(position + 1)} />
          </>
        ) : null}
        <IconButton icon="download" label={t("sftp.download")} onClick={onDownload} />
        {menu === undefined ? null : (
          <ActionMenu label={menu.label} items={menu.items} triggerClassName={iconButtonClassName} />
        )}
        <IconButton icon="close" label={t("sftp.close")} onClick={onClose} />
      </div>
    </div>
  );
}

import type { DragEvent } from "react";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";

const buttonClass = "flex size-6 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill focus-visible:outline-accent";

export function PaneToolbar({ title, moving, focusMode, onSwap, onDragStart, onDragEnd, onFocusMode, onClose }: {
  title: string;
  moving: boolean;
  focusMode: boolean;
  onSwap: () => void;
  onDragStart: (event: DragEvent<HTMLButtonElement>) => void;
  onDragEnd: () => void;
  onFocusMode: () => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  const swapLabel = t("splitView.swapPane", { title });
  const focusLabel = t(focusMode ? "splitView.exitFocus" : "splitView.focusPane", { title });
  const closeLabel = t("splitView.closePane", { title });
  return (
    <div className="flex shrink-0 items-center gap-1" data-pane-toolbar>
      <button
        type="button"
        draggable
        aria-pressed={moving}
        aria-label={swapLabel}
        title={swapLabel}
        className={`${buttonClass} cursor-grab ${moving ? "bg-select-fill text-accent" : ""}`}
        onClick={onSwap}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
      >
        <Icon name="movePane" className="size-3.5" />
      </button>
      <button
        type="button"
        aria-pressed={focusMode}
        aria-label={focusLabel}
        title={focusLabel}
        className={buttonClass}
        onClick={onFocusMode}
      >
        <Icon name="focus" className="size-3.5" />
      </button>
      <button type="button" aria-label={closeLabel} title={closeLabel} className={buttonClass} onClick={onClose}>
        <Icon name="close" className="size-3.5" />
      </button>
    </div>
  );
}

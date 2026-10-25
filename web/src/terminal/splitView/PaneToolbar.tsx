import type { DragEvent } from "react";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";

const buttonClass = "flex size-7 shrink-0 items-center justify-center rounded-[7px] text-ink-faint hover:bg-hover hover:text-ink focus-visible:outline-accent";

// Moving and focusing a pane are shown with the pointer on its header (or while in use), so the
// header keeps the design's three actions: search, more, close.
const revealOnHeaderHover = "opacity-0 group-hover/header:opacity-100 group-focus-within/header:opacity-100 pointer-coarse:opacity-100";

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
    <div className="flex shrink-0 items-center gap-0.5" data-pane-toolbar>
      <button
        type="button"
        draggable
        aria-pressed={moving}
        aria-label={swapLabel}
        title={swapLabel}
        className={`${buttonClass} cursor-grab ${moving ? "bg-select-fill text-accent" : revealOnHeaderHover}`}
        onClick={onSwap}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
      >
        <Icon name="movePane" className="size-[15px]" />
      </button>
      <button
        type="button"
        aria-pressed={focusMode}
        aria-label={focusLabel}
        title={focusLabel}
        className={`${buttonClass} ${focusMode ? "" : revealOnHeaderHover}`}
        onClick={onFocusMode}
      >
        <Icon name="focus" className="size-[15px]" />
      </button>
      <button type="button" aria-label={closeLabel} title={closeLabel} className={buttonClass} onClick={onClose}>
        <Icon name="close" className="size-[15px]" />
      </button>
    </div>
  );
}

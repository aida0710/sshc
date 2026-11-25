import type { DragEvent } from "react";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { IconButton } from "../../ui/IconButton";


// Moving and focusing a pane are shown with the pointer on its header (or while in use), so the
// header keeps the design's three actions: search, more, close.
const revealOnHeaderHover = "opacity-0 group-hover/header:opacity-100 group-focus-within/header:opacity-100 mobile-layout:opacity-100 [@media(any-hover:none)]:opacity-100";

export function PaneToolbar({ title, moving, focusMode, onSwap, onDragStart, onDragEnd, onFocusMode, onRemove }: {
  title: string;
  moving: boolean;
  focusMode: boolean;
  onSwap: () => void;
  onDragStart: (event: DragEvent<HTMLButtonElement>) => void;
  onDragEnd: () => void;
  onFocusMode: () => void;
  onRemove: () => void;
}) {
  const t = useTranslate();
  const swapLabel = t("splitView.swapPane", { title });
  const focusLabel = t(focusMode ? "splitView.exitFocus" : "splitView.focusPane", { title });
  // The session goes on outside the split, so the button does not say close.
  const removeLabel = t("splitView.removePane", { title });
  return (
    <div className="flex shrink-0 items-center gap-0.5" data-pane-toolbar>
      <IconButton label={swapLabel} size="toolbar" tone="plain" className={`cursor-grab ${moving ? "bg-select-fill text-accent" : `text-ink-faint hover:text-ink ${revealOnHeaderHover}`}`}
        draggable
        aria-pressed={moving}
        onClick={onSwap}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}>
        <Icon name="movePane" className="size-[15px]" />
      </IconButton>
      <IconButton label={focusLabel} size="toolbar" tone="faint" className={focusMode ? "" : revealOnHeaderHover}
        aria-pressed={focusMode}
        onClick={onFocusMode}>
        <Icon name="focus" className="size-[15px]" />
      </IconButton>
      <IconButton label={removeLabel} size="toolbar" tone="faint" onClick={onRemove}>
        <Icon name="close" className="size-[15px]" />
      </IconButton>
    </div>
  );
}

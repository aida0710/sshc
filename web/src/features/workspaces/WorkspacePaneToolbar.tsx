import type { DragEvent } from "react";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";

const paneToolbarButtonClass =
  "flex size-6 shrink-0 items-center justify-center rounded text-ink-muted hover:bg-select-fill";

// The strip above one pane of a multi-pane workspace: a handle to move the
// pane, its label, and buttons for focus mode and taking the pane out.
// Every button stops the pointer from reaching the pane, which would
// otherwise focus the pane before the button acts.
export function WorkspacePaneToolbar({
  paneLabel,
  moving,
  inFocusMode,
  onPickToMove,
  onMoveDragStart,
  onCancelMove,
  onToggleFocusMode,
  onDetach,
}: {
  paneLabel: string;
  moving: boolean;
  inFocusMode: boolean;
  onPickToMove: () => void;
  onMoveDragStart: (event: DragEvent<HTMLButtonElement>) => void;
  onCancelMove: () => void;
  onToggleFocusMode: () => void;
  onDetach: () => void;
}) {
  const t = useTranslate();
  const focusModeLabel = t(inFocusMode ? "workspace.exitFocusMode" : "workspace.focusMode", { alias: paneLabel });
  return (
    <div data-pane-toolbar className="flex h-7 shrink-0 items-center gap-1 border-b border-line bg-toolbar px-1">
      <button
        type="button"
        draggable
        aria-pressed={moving}
        aria-label={t(moving ? "workspace.movePanePicked" : "workspace.movePane", { alias: paneLabel })}
        title={t("workspace.movePane", { alias: paneLabel })}
        className={`${paneToolbarButtonClass} cursor-grab text-xs active:cursor-grabbing`}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => {
          event.stopPropagation();
          onPickToMove();
        }}
        onDragStart={onMoveDragStart}
        onDragEnd={onCancelMove}
        onKeyDown={(event) => {
          if (event.key === "Escape") onCancelMove();
        }}
      >
        <Icon name="movePane" className="size-3.5" />
      </button>
      <span className="min-w-0 grow truncate text-xs font-medium text-ink-muted">{paneLabel}</span>
      <button
        type="button"
        aria-pressed={inFocusMode}
        aria-label={focusModeLabel}
        title={focusModeLabel}
        className={`${paneToolbarButtonClass} text-xs`}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => {
          event.stopPropagation();
          onToggleFocusMode();
        }}
      >
        <Icon name="focus" className="size-3.5" />
      </button>
      <button
        type="button"
        aria-label={t("workspace.detachPane")}
        title={t("workspace.detachPane")}
        className={`${paneToolbarButtonClass} text-sm`}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => {
          event.stopPropagation();
          onDetach();
        }}
      >
        <Icon name="close" className="size-3.5" />
      </button>
    </div>
  );
}

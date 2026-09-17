import { useRef, useState } from "react";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { Button } from "../../ui/surface";
import { useDismissibleLayer } from "../../ui/useDismissibleLayer";
import type { SavedWorkspace } from "./api";
import { MAX_WORKSPACE_PANES } from "./layout";

// The strip above a multi-pane workspace: its name and pane count, and a
// menu for broadcasting, leaving focus mode and the saved layouts.
export function WorkspaceMenu({
  displayName,
  paneCount,
  canBroadcast,
  onBroadcast,
  inFocusMode,
  onExitFocusMode,
  saved,
  selected,
  onSelect,
  canSave,
  onSave,
  onReopen,
  onDelete,
}: {
  displayName: string;
  paneCount: number;
  canBroadcast: boolean;
  onBroadcast: () => void;
  inFocusMode: boolean;
  onExitFocusMode: () => void;
  saved: SavedWorkspace[];
  // The id of the saved layout the menu works on; "" means a new one.
  selected: string;
  onSelect: (id: string) => void;
  canSave: boolean;
  onSave: () => void;
  onReopen: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  useDismissibleLayer({
    open,
    containerRefs: [container],
    onDismiss: () => setOpen(false),
  });
  return (
    <div data-desktop-workspace-controls className="hidden h-8 shrink-0 items-center gap-2 border-b border-line bg-toolbar px-2 md:flex">
      {paneCount > 0 ? (
        <div className="flex min-w-0 items-center gap-2">
          <span className="max-w-52 truncate text-[11px] font-semibold text-ink">{displayName}</span>
          <span className="whitespace-nowrap text-[11px] text-ink-faint">{t("workspace.groupCount", { count: String(paneCount) })}</span>
        </div>
      ) : null}
      <div
        ref={container}
        className="group relative ml-auto"
      >
        <button type="button" aria-label={t("workspace.actions")} aria-expanded={open} title={t("workspace.actions")} onClick={() => setOpen((current) => !current)} className="flex size-6 items-center justify-center rounded text-ink-muted hover:bg-select-fill hover:text-ink">
          <Icon name="moreHorizontal" className="size-3.5" />
        </button>
        {open ? <div className="absolute right-0 top-[calc(100%+0.35rem)] z-30 w-80 rounded border border-control-line bg-card p-3 shadow-xl">
          <div className="mb-3 flex flex-wrap gap-2 border-b border-hairline pb-3">
            <Button disabled={!canBroadcast} onClick={() => { setOpen(false); onBroadcast(); }}>{t("workspace.broadcastCommand")}</Button>
            {inFocusMode ? <Button onClick={() => { setOpen(false); onExitFocusMode(); }}>{t("workspace.exitFocusMode")}</Button> : null}
          </div>
          <h2 className="text-sm font-semibold text-ink">{t("workspace.savedLayouts")}</h2>
          <p className="mt-1 text-xs leading-5 text-ink-muted">{t("workspace.savedDescription", { count: MAX_WORKSPACE_PANES })}</p>
          <select aria-label={t("workspace.saved")} value={selected} onChange={(event) => onSelect(event.target.value)} className="mt-3 w-full rounded border border-control-line bg-control px-2 py-1.5 text-xs">
            <option value="">{t("workspace.new")}</option>
            {saved.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button disabled={!canSave} onClick={onSave}>{t("workspace.save")}</Button>
            <Button disabled={selected === ""} onClick={() => onReopen(selected)}>{t("workspace.reopen")}</Button>
            <button disabled={selected === ""} className="px-2 text-xs text-danger disabled:opacity-40" onClick={() => onDelete(selected)}>{t("workspace.delete")}</button>
          </div>
        </div> : null}
      </div>
    </div>
  );
}

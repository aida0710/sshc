import type { DragEvent as ReactDragEvent, KeyboardEvent as ReactKeyboardEvent } from "react";
import { useTranslate } from "../i18n/context";
import { draggableCursor } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { SortButton } from "../ui/tableSort";
import type { RemoteEntry } from "./api";
import { EntryThumbnail } from "./EntryThumbnail";
import { gridArrowTarget, gridColumnCount, isGridArrowKey } from "./gridFocus";
import { EntryName, entrySize } from "./sftpEntryLook";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import { sortColumnLabelKeys } from "./sftpMessageKeys";
import { useEntryRowProps } from "./useEntryRowProps";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";

const cellButton = "flex w-full min-w-0 cursor-[inherit] flex-col gap-1 rounded-lg p-1.5 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-accent";
const pictureBox = "aspect-square w-full rounded-md bg-surface-subtle";

// The head of the grid on a pointer's screen: what the table's header offers, with no columns under it. A phone has its
// own head (SFTPPhoneListHeader).
function SFTPEntryGridHeader({ model, sort, columns, onSort }: {
  model: SFTPEntryListModel;
  sort: SFTPSortState;
  columns: readonly SFTPSort[];
  onSort: (key: SFTPSort) => void;
}) {
  const t = useTranslate();
  return (
    <div className="sticky top-0 z-[2] flex flex-wrap items-center gap-x-3 gap-y-1 bg-sidebar py-2 pl-3.5 pr-2 text-xs text-ink-faint">
      <input
        ref={model.selectAll}
        type="checkbox"
        aria-label={t("sftp.selectAll")}
        checked={model.allDisplayedSelected}
        onChange={model.toggleAllDisplayed}
        className="size-4 accent-accent"
      />
      {columns.map((column) => (
        <SortButton key={column} column={column} activeColumn={sort.key} direction={sort.direction} onSort={onSort}>
          {t(sortColumnLabelKeys[column])}
        </SortButton>
      ))}
    </div>
  );
}

// The rows as a grid of pictures, for a folder of photos. Every cell is wired like a row of the other lists
// (useEntryRowProps), so a tap, a long press, a click, a right click and the keyboard do the same things here. Only the
// arrow keys differ, since a grid has neighbours on four sides.
export function SFTPEntryGrid({
  model, entries, sort, sortColumns, onSort, mobileInteraction, busy, locked, parentRowVisible, draggable, onDragStart, entryContext, thumbnailAlias,
}: {
  model: SFTPEntryListModel;
  entries: RemoteEntry[];
  sort: SFTPSortState;
  sortColumns: readonly SFTPSort[];
  onSort: (key: SFTPSort) => void;
  mobileInteraction: boolean;
  busy: boolean;
  locked: boolean;
  parentRowVisible: boolean;
  draggable: (entry: RemoteEntry) => boolean;
  onDragStart: ((event: ReactDragEvent<HTMLElement>, entry: RemoteEntry) => void) | undefined;
  entryContext: ((entry: RemoteEntry) => string) | undefined;
  thumbnailAlias: string | null;
}) {
  const t = useTranslate();
  const { selectedPaths, rowContextMenu, activate, rowKeys, moveFocusToRow } = model;
  const { entryCheckboxProps, entryButtonProps, parentButtonProps } = useEntryRowProps({ model, busy, locked });
  const choosing = selectedPaths.size > 0;

  // Arrow keys are taken here, before the list's own handling of up and down moves one row along the display order.
  function moveWithArrows(event: ReactKeyboardEvent<HTMLUListElement>) {
    if (!isGridArrowKey(event.key) || busy) return;
    const cell = event.target instanceof Element ? event.target.closest("[data-row-key]") : null;
    const from = rowKeys.indexOf(cell?.getAttribute("data-row-key") ?? "");
    if (from < 0) return;
    event.preventDefault();
    event.stopPropagation();
    const target = gridArrowTarget({ key: event.key, from, count: rowKeys.length, columns: gridColumnCount(event.currentTarget) });
    moveFocusToRow(rowKeys[target] ?? rowKeys[from] ?? "", event);
  }

  return (
    <>
      {mobileInteraction ? null : <SFTPEntryGridHeader model={model} sort={sort} columns={sortColumns} onSort={onSort} />}
      <ul
        aria-label={t("sftp.entries")}
        onKeyDown={moveWithArrows}
        className={`grid gap-1 p-1.5 ${mobileInteraction ? "grid-cols-[repeat(auto-fill,minmax(6.5rem,1fr))]" : "grid-cols-[repeat(auto-fill,minmax(8rem,1fr))]"}`}
      >
        {parentRowVisible ? (
          <li data-row-key={parentRowKey} className="min-w-0 rounded-lg hover:bg-hover">
            <button type="button" {...parentButtonProps()} className={cellButton}>
              <span className={`flex items-center justify-center ${pictureBox}`}><Icon name="folder" className="size-10 text-accent" /></span>
              <span aria-hidden="true" className="font-mono text-xs leading-4 text-ink">..</span>
              <span className="sr-only">{t("sftp.parentDirectory")}</span>
            </button>
          </li>
        ) : null}
        {entries.map((entry) => {
          const selected = selectedPaths.has(entry.path);
          return (
            <li
              key={entry.path}
              data-row-key={entry.path}
              // On a phone a held finger starts choosing. A draggable cell would hand the press to the browser's drag
              // instead, and the long press would be cancelled.
              draggable={!mobileInteraction && draggable(entry)}
              onDragStart={(event) => onDragStart?.(event, entry)}
              onDoubleClick={() => activate(entry)}
              onContextMenu={(event) => rowContextMenu(event, entry)}
              className={`group relative min-w-0 rounded-lg transition-colors ${!mobileInteraction && draggable(entry) ? draggableCursor : "cursor-default"} ${selected ? "bg-select-fill ring-2 ring-inset ring-accent" : "hover:bg-hover"}`}
            >
              <button
                type="button"
                {...entryButtonProps(entry)}
                className={`${cellButton} ${mobileInteraction ? "touch-pan-y select-none active:bg-select-fill" : ""}`}
              >
                <EntryThumbnail
                  alias={thumbnailAlias}
                  entry={entry}
                  boxClassName={pictureBox}
                  iconClassName="size-10"
                  folderColour={mobileInteraction ? "text-ink-muted" : "text-accent"}
                />
                <span title={entry.name} className="line-clamp-2 break-all text-xs leading-4 text-ink"><EntryName entry={entry} /></span>
                <span className="truncate text-[11px] leading-[14px] text-ink-muted">{entryContext === undefined ? entrySize(entry) : entryContext(entry)}</span>
              </button>
              {mobileInteraction ? (
                choosing ? (
                  <label className="absolute left-0 top-0 flex size-11 items-center justify-center">
                    <input type="checkbox" {...entryCheckboxProps(entry)} className="size-5 accent-accent" />
                  </label>
                ) : null
              ) : (
                <input
                  type="checkbox"
                  {...entryCheckboxProps(entry)}
                  onDoubleClick={(event) => event.stopPropagation()}
                  className={`absolute left-2.5 top-2.5 size-4 accent-accent transition-opacity group-hover:opacity-100 focus-visible:opacity-100 ${choosing ? "opacity-100" : "opacity-0"}`}
                />
              )}
            </li>
          );
        })}
      </ul>
    </>
  );
}

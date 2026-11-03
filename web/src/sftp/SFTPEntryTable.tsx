import type { DragEvent as ReactDragEvent } from "react";
import { useLanguage, useTranslate } from "../i18n/context";
import { draggableCursor } from "../ui/dragAndDropLook";
import { formatDateTime } from "../ui/format";
import { Icon } from "../ui/icons";
import { SortableTableHeader } from "../ui/tableSort";
import type { RemoteEntry } from "./api";
import { EntryThumbnail } from "./EntryThumbnail";
import { EntryIcon, EntryName, entrySize, entryTypeLabelKey } from "./sftpEntryLook";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import { useEntryRowProps } from "./useEntryRowProps";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";

// The cells of the wide table: 7px above and below each row, as in the design.
const headerCell = "px-2 pb-2 pt-2.5 font-normal";
const bodyCell = "px-2 py-[7px]";
// With a thumbnail the name cell keeps the height of a line of text: the 24px box takes what the padding gives up, so
// that a picture does not make the table taller.
const thumbnailCell = "px-2 py-[5px]";
// In the wide table only the name grows: every other column is as wide as what it holds, with room after it.
const fittedColumn = "w-px whitespace-nowrap pr-7";

function ParentRowLabel({ iconBoxClassName }: { iconBoxClassName: string }) {
  const t = useTranslate();
  return (
    <>
      <span className={`flex shrink-0 justify-center ${iconBoxClassName}`}><Icon name="folder" className="size-[15px] text-accent" /></span>
      <span aria-hidden="true" className="font-mono">..</span>
      <span className="sr-only">{t("sftp.parentDirectory")}</span>
    </>
  );
}

// The rows on a pointer's screen, a table with sortable columns. A pane, however narrow, keeps the table and
// scrolls it sideways rather than dropping columns.
export function SFTPEntryTable({
  model,
  entries,
  sort,
  onSort,
  compact,
  showOwnership,
  busy,
  locked,
  parentRowVisible,
  draggable,
  onDragStart,
  entryContext,
  thumbnailAlias,
}: {
  model: SFTPEntryListModel;
  entries: RemoteEntry[];
  sort: SFTPSortState;
  onSort: (key: SFTPSort) => void;
  compact: boolean;
  showOwnership: boolean;
  busy: boolean;
  locked: boolean;
  parentRowVisible: boolean;
  draggable: (entry: RemoteEntry) => boolean;
  onDragStart: ((event: ReactDragEvent<HTMLElement>, entry: RemoteEntry) => void) | undefined;
  entryContext: ((entry: RemoteEntry) => string) | undefined;
  thumbnailAlias: string | null;
}) {
  const t = useTranslate();
  const { locale } = useLanguage();
  const tableOwnership = showOwnership && !compact;
  // A narrow pane fixes its columns (colgroup); the wide table fits all but the name to what they hold.
  const fitted = compact ? "" : fittedColumn;
  const {
    selectedPaths, allDisplayedSelected, selectAll, activate, toggleAllDisplayed, rowContextMenu,
  } = model;
  const { entryCheckboxProps, entryButtonProps, parentButtonProps } = useEntryRowProps({ model, busy, locked });
  const rowCursor = (entry: RemoteEntry) => (draggable(entry) ? draggableCursor : "cursor-default");
  const showsThumbnails = thumbnailAlias !== null;

  return (
    <table className={`w-full text-left text-sm ${compact ? "table-fixed" : "min-w-[44rem]"}`}>
      {compact ? <colgroup><col className="w-9" /><col /><col className="w-36" /><col className="w-20" /><col className="w-20" /></colgroup> : null}
      <thead className="sticky top-0 z-[1] bg-sidebar text-xs text-ink-faint"><tr>
        <th scope="col" className="w-9 py-2 pl-3.5 pr-2 font-normal">
          <input
            ref={selectAll}
            type="checkbox"
            aria-label={t("sftp.selectAll")}
            checked={allDisplayedSelected}
            onChange={toggleAllDisplayed}
            className="size-4 accent-accent"
          />
        </th>
        <SortableTableHeader column="name" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={headerCell}>{t("sftp.name")}</SortableTableHeader>
        <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} ${fitted}`}>{t("sftp.modified")}</SortableTableHeader>
        <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} ${fitted} text-right`} buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
        <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} ${compact ? "w-24 whitespace-nowrap" : fittedColumn}`}>{t("sftp.type")}</SortableTableHeader>
        {tableOwnership ? <>
        <SortableTableHeader column="uid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} ${fittedColumn}`}>{t("sftp.uid")}</SortableTableHeader>
        <SortableTableHeader column="gid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} ${fittedColumn}`}>{t("sftp.gid")}</SortableTableHeader>
        </> : null}
        {compact ? null : <th scope="col" className={`${headerCell} ${fittedColumn}`}>{t("sftp.permissions")}</th>}
      </tr></thead>
      <tbody>
        {parentRowVisible ? (
          <tr data-row-key={parentRowKey} className="border-t border-line hover:bg-hover">
            <td className={`${bodyCell} pl-3.5`} colSpan={compact ? 5 : tableOwnership ? 8 : 6}>
              <button
                type="button"
                {...parentButtonProps()}
                className="flex w-full items-center gap-2 rounded text-left text-[13px] leading-5 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
              >
                <ParentRowLabel iconBoxClassName={showsThumbnails ? "w-6" : ""} />
              </button>
            </td>
          </tr>
        ) : null}
        {entries.map((entry) => (
          <tr
            key={entry.path}
            data-row-key={entry.path}
            aria-selected={selectedPaths.has(entry.path)}
            onDoubleClick={() => activate(entry)}
            onContextMenu={(event) => rowContextMenu(event, entry)}
            draggable={draggable(entry)}
            onDragStart={(event) => onDragStart?.(event, entry)}
            className={`${rowCursor(entry)} border-t border-line transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill" : "hover:bg-hover"}`}
          >
            <td className={`${bodyCell} w-9 pl-3.5`}>
              <input type="checkbox" {...entryCheckboxProps(entry)} onDoubleClick={(event) => event.stopPropagation()} />
            </td>
            <td className={`${showsThumbnails ? thumbnailCell : bodyCell} max-w-64`}>
              <button
                type="button"
                {...entryButtonProps(entry)}
                className={`flex w-full min-w-0 items-center gap-2 rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent ${rowCursor(entry)}`}
              >
                {showsThumbnails
                  ? <EntryThumbnail alias={thumbnailAlias} entry={entry} boxClassName="size-6 rounded-[3px]" iconClassName="size-[15px]" />
                  : <EntryIcon entry={entry} className="size-[15px] shrink-0" />}
                <span className="min-w-0 grow">
                  <span className="block truncate text-[13px] leading-5 text-ink"><EntryName entry={entry} /></span>
                  {entryContext === undefined ? null : <span className="block truncate font-mono text-[10px] leading-3 text-ink-muted">{entryContext(entry)}</span>}
                </span>
              </button>
            </td>
            <td className={`${bodyCell} ${fitted} truncate whitespace-nowrap text-xs text-ink-faint`} title={new Date(entry.modifiedAt).toLocaleString(locale)}>{formatDateTime(entry.modifiedAt, locale)}</td>
            <td className={`${bodyCell} ${fitted} whitespace-nowrap text-right font-mono text-xs text-ink-muted`}>{entrySize(entry)}</td>
            <td className={`${bodyCell} ${compact ? "w-24" : fittedColumn} whitespace-nowrap text-xs text-ink-muted`}>{t(entryTypeLabelKey(entry))}</td>
            {tableOwnership ? <>
            <td className={`${bodyCell} ${fittedColumn} font-mono text-xs text-ink-muted`}>{entry.uid ?? "—"}</td>
            <td className={`${bodyCell} ${fittedColumn} font-mono text-xs text-ink-muted`}>{entry.gid ?? "—"}</td>
            </> : null}
            {compact ? null : <td className={`${bodyCell} ${fittedColumn} font-mono text-xs text-ink-muted`}>{entry.mode}</td>}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

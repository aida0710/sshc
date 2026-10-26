import type { ComponentPropsWithoutRef, ComponentPropsWithRef, DragEvent as ReactDragEvent } from "react";
import { useLanguage, useTranslate } from "../i18n/context";
import { draggableCursor } from "../ui/dragAndDropLook";
import { Icon } from "../ui/icons";
import { SortableTableHeader } from "../ui/tableSort";
import type { RemoteEntry } from "./api";
import { formatBytes, formatDateTime } from "../ui/format";
import { entryKind } from "./entryKind";
import { entryTypeLabelKeys } from "./sftpMessageKeys";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";

// The list shows sizes in units people read at a glance; the exact byte
// count stays in the details dialog.
function entrySize(entry: RemoteEntry): string {
  return entryKind(entry) === "file" ? formatBytes(entry.size) : "—";
}

// The cells of the wide table: 7px above and below each row, as in the design.
const headerCell = "px-2 pb-2 pt-2.5 font-normal";
const bodyCell = "px-2 py-[7px]";

function entryIcon(entry: RemoteEntry) {
  return entryKind(entry) === "directory" ? "folder" : "file";
}

// A symlink is named with where it points, as `ls -l` does.
function EntryName({ entry }: { entry: RemoteEntry }) {
  const t = useTranslate();
  if (entry.type !== "symlink") return <>{entry.name}</>;
  const target = entry.targetType === undefined ? t("sftp.brokenLink", { target: entry.linkTarget ?? "" }) : entry.linkTarget ?? "";
  return <>{entry.name}<span className="font-normal text-ink-muted">{` → ${target}`}</span></>;
}

function ParentRowLabel() {
  const t = useTranslate();
  return (
    <>
      <Icon name="folder" className="size-[15px] shrink-0 text-accent" />
      <span aria-hidden="true" className="font-mono">..</span>
      <span className="sr-only">{t("sftp.parentDirectory")}</span>
    </>
  );
}

// The rows themselves: a table with sortable columns where there is room, and
// a two-line list where there is not. Both read the same model.
export function SFTPEntryList({
  model,
  entries,
  sort,
  onSort,
  mobileInteraction,
  compact = false,
  showOwnership = true,
  busy,
  locked = false,
  parentRowVisible,
  draggable = () => false,
  onDragStart,
  entryContext,
}: {
  model: SFTPEntryListModel;
  entries: RemoteEntry[];
  sort: SFTPSortState;
  onSort: (key: SFTPSort) => void;
  mobileInteraction: boolean;
  compact?: boolean;
  showOwnership?: boolean;
  busy: boolean;
  locked?: boolean;
  parentRowVisible: boolean;
  draggable?: (entry: RemoteEntry) => boolean;
  onDragStart?: (event: ReactDragEvent<HTMLElement>, entry: RemoteEntry) => void;
  // Where an entry lives when the rows are not one directory, such as search
  // results. Shown under the name instead of the mode.
  entryContext?: ((entry: RemoteEntry) => string) | undefined;
}) {
  const t = useTranslate();
  const { locale } = useLanguage();
  const tableOwnership = showOwnership && !compact;
  const {
    selectedPaths, activeRowKey, allDisplayedSelected, selectAll, registerRow, setFocusedKey,
    activate, openParent, clickEntry, toggleSelection, toggleAllDisplayed,
    rowContextMenu, beginLongPress, trackLongPress, cancelLongPress,
  } = model;
  const rowDraggable = (entry: RemoteEntry) => !mobileInteraction && draggable(entry);
  const rowCursor = (entry: RemoteEntry) => (rowDraggable(entry) ? draggableCursor : "cursor-default");
  // Both layouts wire their rows through these three, so that focus, selection
  // and the long press work the same whichever layout is on screen. Only the
  // markup around them differs.
  const parentButtonProps: ComponentPropsWithRef<"button"> = {
    ref: (node) => { registerRow(parentRowKey, node); },
    tabIndex: activeRowKey === parentRowKey ? 0 : -1,
    disabled: busy || locked,
    onFocus: () => setFocusedKey(parentRowKey),
    onClick: openParent,
  };
  const entryCheckboxProps = (entry: RemoteEntry): ComponentPropsWithoutRef<"input"> => ({
    "aria-label": t("sftp.selectEntry", { name: entry.name }),
    checked: selectedPaths.has(entry.path),
    tabIndex: activeRowKey === entry.path ? 0 : -1,
    disabled: busy,
    onChange: () => toggleSelection(entry),
    className: "size-4 accent-accent",
  });
  // A locked list still lets a row be selected; the model refuses to open it.
  const entryButtonProps = (entry: RemoteEntry): ComponentPropsWithRef<"button"> => ({
    ref: (node) => { registerRow(entry.path, node); },
    "aria-label": entry.name,
    "aria-pressed": selectedPaths.has(entry.path),
    tabIndex: activeRowKey === entry.path ? 0 : -1,
    disabled: busy,
    onFocus: () => setFocusedKey(entry.path),
    onClick: (event) => clickEntry(entry, event),
    onPointerDown: (event) => beginLongPress(event, entry),
    onPointerMove: trackLongPress,
    onPointerUp: cancelLongPress,
    onPointerCancel: cancelLongPress,
  });

  // Touch devices get two-line rows with targets of at least 44px. Every
  // pointer-driven pane, however narrow, keeps the full table and scrolls it
  // sideways rather than dropping columns.
  if (mobileInteraction) {
    return (
      <ul aria-label={t("sftp.entries")} className="divide-y divide-line/40">
        {parentRowVisible ? (
          <li data-row-key={parentRowKey}>
            <button
              type="button"
              {...parentButtonProps}
              className="flex min-h-11 w-full items-center gap-2.5 px-3 py-1.5 text-left text-base hover:bg-hover"
            >
              <ParentRowLabel />
            </button>
          </li>
        ) : null}
        {entries.map((entry) => (
          <li
            key={entry.path}
            data-row-key={entry.path}
            className={`flex items-center transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill/75" : ""}`}
            onContextMenu={(event) => rowContextMenu(event, entry)}
            draggable={rowDraggable(entry)}
            onDragStart={(event) => onDragStart?.(event, entry)}
          >
            <label className="flex min-h-11 w-11 shrink-0 self-stretch items-center justify-center">
              <input type="checkbox" {...entryCheckboxProps(entry)} className="size-5 accent-accent" />
            </label>
            <button
              type="button"
              {...entryButtonProps(entry)}
              className="flex min-h-14 min-w-0 flex-1 touch-pan-y select-none items-center gap-2.5 px-2.5 py-1.5 text-left hover:bg-hover active:bg-select-fill"
            >
              <Icon name={entryIcon(entry)} className="size-5 shrink-0 text-ink-muted" />
              <span className="min-w-0 flex-1">
                <span title={entry.type === "symlink" ? `${entry.name} → ${entry.linkTarget ?? ""}` : entry.name} className="block truncate font-mono text-base font-medium leading-5 text-ink"><EntryName entry={entry} /></span>
                <span className="mt-0.5 flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs leading-4 text-ink-muted">
                  <span title={entryContext === undefined ? entry.mode : entryContext(entry)} className="max-w-full truncate font-mono">{entryContext === undefined ? entry.mode : entryContext(entry)}</span>
                  {!showOwnership || entry.uid === undefined ? null : <span title={t("sftp.ownerIds", { uid: entry.uid, gid: entry.gid ?? "—" })} aria-label={t("sftp.ownerIds", { uid: entry.uid, gid: entry.gid ?? "—" })} className="max-w-full truncate font-mono">{entry.uid}:{entry.gid ?? "—"}</span>}
                  <span className="whitespace-nowrap">{entrySize(entry)}</span>
                  <time title={new Date(entry.modifiedAt).toLocaleString(locale)} className="whitespace-nowrap" dateTime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleDateString(locale)}</time>
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    );
  }

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
        <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={headerCell}>{t("sftp.modified")}</SortableTableHeader>
        <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} text-right`} buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
        <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={`${headerCell} w-24 whitespace-nowrap`}>{t("sftp.type")}</SortableTableHeader>
        {tableOwnership ? <>
        <SortableTableHeader column="uid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={headerCell}>{t("sftp.uid")}</SortableTableHeader>
        <SortableTableHeader column="gid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className={headerCell}>{t("sftp.gid")}</SortableTableHeader>
        </> : null}
        {compact ? null : <th scope="col" className={`${headerCell} w-28 whitespace-nowrap`}>{t("sftp.permissions")}</th>}
      </tr></thead>
      <tbody>
        {parentRowVisible ? (
          <tr data-row-key={parentRowKey} className="border-t border-line hover:bg-hover">
            <td className={`${bodyCell} pl-3.5`} colSpan={compact ? 5 : tableOwnership ? 8 : 6}>
              <button
                type="button"
                {...parentButtonProps}
                className="flex w-full items-center gap-2 rounded text-left text-[13px] leading-5 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
              >
                <ParentRowLabel />
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
            draggable={rowDraggable(entry)}
            onDragStart={(event) => onDragStart?.(event, entry)}
            className={`${rowCursor(entry)} border-t border-line transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill" : "hover:bg-hover"}`}
          >
            <td className={`${bodyCell} w-9 pl-3.5`}>
              <input type="checkbox" {...entryCheckboxProps(entry)} onDoubleClick={(event) => event.stopPropagation()} />
            </td>
            <td className={`${bodyCell} max-w-64`}>
              <button
                type="button"
                {...entryButtonProps(entry)}
                className={`flex w-full min-w-0 items-center gap-2 rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent ${rowCursor(entry)}`}
              >
                <Icon name={entryIcon(entry)} className={`size-[15px] shrink-0 ${entryKind(entry) === "directory" ? "text-accent" : "text-ink-muted"}`} />
                <span className="min-w-0 grow">
                  <span className="block truncate text-[13px] leading-5 text-ink"><EntryName entry={entry} /></span>
                  {entryContext === undefined ? null : <span className="block truncate font-mono text-[10px] leading-3 text-ink-muted">{entryContext(entry)}</span>}
                </span>
              </button>
            </td>
            <td className={`${bodyCell} truncate whitespace-nowrap text-xs text-ink-faint`} title={new Date(entry.modifiedAt).toLocaleString(locale)}>{formatDateTime(entry.modifiedAt, locale)}</td>
            <td className={`${bodyCell} whitespace-nowrap text-right font-mono text-xs text-ink-muted`}>{entrySize(entry)}</td>
            <td className={`${bodyCell} w-24 whitespace-nowrap text-xs text-ink-muted`}>{t(entryTypeLabelKeys[entry.type])}</td>
            {tableOwnership ? <>
            <td className={`${bodyCell} font-mono text-xs text-ink-muted`}>{entry.uid ?? "—"}</td>
            <td className={`${bodyCell} font-mono text-xs text-ink-muted`}>{entry.gid ?? "—"}</td>
            </> : null}
            {compact ? null : <td className={`${bodyCell} w-28 whitespace-nowrap font-mono text-xs text-ink-muted`}>{entry.mode}</td>}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

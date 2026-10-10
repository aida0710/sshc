import type { ComponentPropsWithoutRef, ComponentPropsWithRef, DragEvent as ReactDragEvent } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { SortableTableHeader } from "../ui/tableSort";
import type { RemoteEntry } from "./api";
import { formatBytes } from "../ui/format";
import { entryKind } from "./entryKind";
import { entryTypeLabelKeys } from "./sftpMessageKeys";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";

// The list shows sizes in units people read at a glance; the exact byte
// count stays in the details dialog.
function entrySize(entry: RemoteEntry): string {
  return entryKind(entry) === "file" ? formatBytes(entry.size) : "—";
}

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
      <Icon name="groups" className="size-4 text-ink-muted" />
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
  const {
    selectedPaths, activeRowKey, allDisplayedSelected, selectAll, registerRow, setFocusedKey,
    activate, openParent, clickEntry, toggleSelection, toggleAllDisplayed,
    rowContextMenu, beginLongPress, trackLongPress, cancelLongPress,
  } = model;
  const rowDraggable = (entry: RemoteEntry) => !mobileInteraction && draggable(entry);
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
              className="flex min-h-11 w-full items-center gap-2 px-2 py-1.5 text-left text-sm hover:bg-hover disabled:text-ink-faint md:min-h-8 md:py-0.5"
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
            <label className="flex size-11 shrink-0 items-center justify-center">
              <input type="checkbox" {...entryCheckboxProps(entry)} />
            </label>
            <button
              type="button"
              {...entryButtonProps(entry)}
              className="flex min-h-12 min-w-0 grow touch-pan-y select-none items-center gap-2 px-2 py-2 text-left hover:bg-hover active:bg-select-fill disabled:text-ink-faint"
            >
              <Icon name={entryIcon(entry)} className="size-4 shrink-0 text-ink-muted" />
              <span className="min-w-0 grow">
                <span className="block truncate font-mono text-sm font-medium leading-4 text-ink"><EntryName entry={entry} /></span>
                <span className="mt-0.5 flex min-w-0 gap-2 text-[11px] leading-3 text-ink-muted">
                  <span className="truncate font-mono">{entryContext === undefined ? entry.mode : entryContext(entry)}</span>
                  {!showOwnership || entry.uid === undefined ? null : <span>{t("sftp.ownerIds", { uid: entry.uid, gid: entry.gid ?? "—" })}</span>}
                  <span>{entrySize(entry)}</span>
                  <time className="truncate" dateTime={entry.modifiedAt}>{new Date(entry.modifiedAt).toLocaleString()}</time>
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    );
  }

  return (
    <table className="w-full min-w-[44rem] text-left text-sm">
      <thead className="sticky top-0 bg-toolbar/75 text-xs text-ink-muted"><tr>
        <th scope="col" className="w-9 px-2 py-1.5 md:py-1">
          <input
            ref={selectAll}
            type="checkbox"
            aria-label={t("sftp.selectAll")}
            checked={allDisplayedSelected}
            onChange={toggleAllDisplayed}
            className="size-4 accent-accent"
          />
        </th>
        <SortableTableHeader column="name" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="px-2 py-1.5 md:py-1">{t("sftp.name")}</SortableTableHeader>
        <SortableTableHeader column="modified" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="px-2 py-1.5 md:py-1">{t("sftp.modified")}</SortableTableHeader>
        <SortableTableHeader column="size" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="px-2 py-1.5 text-right md:py-1" buttonClassName="justify-end">{t("sftp.size")}</SortableTableHeader>
        <SortableTableHeader column="type" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="w-24 whitespace-nowrap px-2 py-1.5 md:py-1">{t("sftp.type")}</SortableTableHeader>
        {showOwnership ? <>
        <SortableTableHeader column="uid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="px-2 py-1.5 md:py-1">{t("sftp.uid")}</SortableTableHeader>
        <SortableTableHeader column="gid" activeColumn={sort.key} direction={sort.direction} onSort={onSort} className="px-2 py-1.5 md:py-1">{t("sftp.gid")}</SortableTableHeader>
        </> : null}
        <th scope="col" className="w-28 whitespace-nowrap px-2 py-1.5 md:py-1">{t("sftp.permissions")}</th>
      </tr></thead>
      <tbody>
        {parentRowVisible ? (
          <tr data-row-key={parentRowKey} className="border-t border-line/40 hover:bg-hover/60">
            <td className="px-2 py-1 md:py-0.5" colSpan={showOwnership ? 8 : 6}>
              <button
                type="button"
                {...parentButtonProps}
                className="flex w-full items-center gap-2 rounded py-0.5 text-left text-sm focus:outline-none focus-visible:ring-1 focus-visible:ring-accent disabled:text-ink-faint"
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
            className={`cursor-default border-t border-line/40 transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill/75" : "hover:bg-hover/55"}`}
          >
            <td className="w-9 px-2 py-1 md:py-0.5">
              <input type="checkbox" {...entryCheckboxProps(entry)} onDoubleClick={(event) => event.stopPropagation()} />
            </td>
            <td className="max-w-64 px-2 py-1 md:py-0.5">
              <button
                type="button"
                {...entryButtonProps(entry)}
                className="flex w-full min-w-0 items-center gap-2 rounded text-left focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
              >
                <Icon name={entryIcon(entry)} className="size-4 text-ink-muted" />
                <span className="min-w-0 grow">
                  <span className="block truncate font-mono text-sm font-medium leading-4 text-ink"><EntryName entry={entry} /></span>
                  {entryContext === undefined ? null : <span className="block truncate font-mono text-[10px] leading-3 text-ink-muted">{entryContext(entry)}</span>}
                </span>
              </button>
            </td>
            <td className="whitespace-nowrap px-2 py-1 text-xs text-ink-muted md:py-0.5">{new Date(entry.modifiedAt).toLocaleString()}</td>
            <td className="px-2 py-1 text-right text-xs text-ink-muted md:py-0.5">{entrySize(entry)}</td>
            <td className="w-24 whitespace-nowrap px-2 py-1 text-xs text-ink-muted md:py-0.5">{t(entryTypeLabelKeys[entry.type])}</td>
            {showOwnership ? <>
            <td className="px-2 py-1 font-mono text-xs text-ink-muted md:py-0.5">{entry.uid ?? "—"}</td>
            <td className="px-2 py-1 font-mono text-xs text-ink-muted md:py-0.5">{entry.gid ?? "—"}</td>
            </> : null}
            <td className="w-28 whitespace-nowrap px-2 py-1 font-mono text-xs text-ink-muted md:py-0.5">{entry.mode}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

import { useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { iconAction } from "../ui/form";
import { Icon } from "../ui/icons";
import { nextSort } from "../ui/tableSort";
import { SFTPActionSheet } from "./SFTPActionSheet";
import { SFTPEntryViewSwitch } from "./SFTPEntryViewSwitch";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import { sortColumnLabelKeys } from "./sftpMessageKeys";
import type { SFTPEntryView } from "./sftpPanes";
import { phoneMenuItemClass } from "./SFTPMenuActionList";

function sortArrow({ direction }: SFTPSortState) {
  return direction === "ascending" ? "arrowUp" : "arrowDown";
}

// The head of the list on a phone, which has no table to sort by its columns: the order the rows are in, which opens
// the other orders, how the rows are drawn, and a way to read the folder again.
export function SFTPPhoneListHeader({ sort, columns, onSort, entryView, onEntryViewChange, onRefresh, refreshDisabled }: {
  sort: SFTPSortState;
  columns: readonly SFTPSort[];
  onSort: (key: SFTPSort) => void;
  entryView: SFTPEntryView;
  onEntryViewChange: (view: SFTPEntryView) => void;
  onRefresh: () => void;
  refreshDisabled: boolean;
}) {
  const t = useTranslate();
  const [choosing, setChoosing] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);

  return (
    <div className="flex shrink-0 items-center justify-between border-b border-line/40 pl-1">
      <button
        ref={trigger}
        type="button"
        aria-label={t("sftp.sortedBy", { column: t(sortColumnLabelKeys[sort.key]) })}
        aria-haspopup="dialog"
        aria-expanded={choosing}
        onClick={() => setChoosing(true)}
        className="flex min-h-11 items-center gap-1 rounded-md px-2 text-xs text-ink-muted active:bg-select-fill"
      >
        {t(sortColumnLabelKeys[sort.key])}
        <Icon name={sortArrow(sort)} className="size-3.5 text-accent" />
      </button>
      <div className="flex items-center gap-1">
        <SFTPEntryViewSwitch view={entryView} onChange={onEntryViewChange} touchCompact />
        <button type="button" aria-label={t("sftp.refreshDirectory")} disabled={refreshDisabled} onClick={onRefresh} className={`${iconAction} size-11 active:bg-select-fill`}>
          <Icon name="sync" className="size-4" />
        </button>
      </div>
      {choosing ? (
        <SFTPActionSheet heading={t("sftp.sortMenu")} onDismiss={() => setChoosing(false)} returnFocusRef={trigger}>
          {columns.map((column) => {
            const chosen = column === sort.key;
            const nextDirection = nextSort(sort.key, sort.direction, column).direction;
            return (
              <button
                key={column}
                type="button"
                role="menuitemradio"
                aria-checked={chosen}
                onClick={() => { onSort(column); setChoosing(false); }}
                className={`${phoneMenuItemClass} flex items-center justify-between gap-3`}
              >
                <span>
                  {t(sortColumnLabelKeys[column])}
                  <span className="sr-only">{t(nextDirection === "ascending" ? "table.sortAscending" : "table.sortDescending")}</span>
                </span>
                {chosen ? <Icon name={sortArrow(sort)} className="size-4 text-accent" /> : null}
              </button>
            );
          })}
        </SFTPActionSheet>
      ) : null}
    </div>
  );
}

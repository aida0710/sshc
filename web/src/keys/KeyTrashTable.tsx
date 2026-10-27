import { useState } from "react";
import type { TrashListResponse } from "./api";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { tableHeadCell, tableHeadRow } from "../ui/form";
import { Button } from "../ui/surface";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";
import { describeBlockers } from "./labels";

type TrashSort = "files" | "age" | "status";
type TrashEntry = TrashListResponse["entries"][number];

function trashFilesText(entry: TrashEntry): string {
  return entry.files.map((file) => file.originalRelativePath).join(", ");
}

function trashStatusText(entry: TrashEntry, t: Translate): string {
  return entry.restorable ? t("keys.restorable") : describeBlockers(entry.blockers, t);
}

// KeyTrashTable is what the trash entry of the left list opens, in place of the keys: restorable entries
// come back whole, the rest wait out the retention or are purged after an explicit confirmation. Like the
// keys, it is a table where its container (keytable) has room and one card per entry where it has not.
export function KeyTrashTable({ trash, onRestore, onPurge }: {
  trash: TrashListResponse;
  onRestore: (entryId: string) => Promise<void>;
  // Resolves once the entry is gone; the confirmation closes on its own.
  onPurge: (entryId: string) => Promise<boolean>;
}) {
  const t = useTranslate();
  const trashSort = useTableSort<TrashSort>("files");
  // 並べ替えは、セルに出す文と同じ文で比べる。
  const trashSortValue = (entry: TrashEntry, column: TrashSort): SortValue => {
    switch (column) {
      case "files": return trashFilesText(entry);
      case "age": return entry.ageDays;
      case "status": return trashStatusText(entry, t);
    }
  };
  const [pendingPurge, setPendingPurge] = useState("");
  const [purgeError, setPurgeError] = useState("");
  // 失敗の文は開いている確認ダイアログのものなので、閉じたら消す。残すと、開き直した
  // ダイアログにまだ押していない操作の失敗が出る。
  function closePurge() {
    setPendingPurge("");
    setPurgeError("");
  }
  const cell = "border-line @xl/keytable:table-cell @xl/keytable:border-t @xl/keytable:px-2.5 @xl/keytable:py-[9px]";
  return (
    <>
      <p className="mb-3 text-xs leading-[18px] text-ink-faint">{t("keys.trashNote")}</p>
      <table className="block w-full text-left text-[13px] @xl/keytable:table @xl/keytable:table-fixed">
        <caption className="sr-only">{t("keys.trashCaption")}</caption>
        <thead className="hidden @xl/keytable:table-header-group">
          <tr className={tableHeadRow}>
            <SortableTableHeader column="files" {...trashSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
              {t("keys.colFiles")}
            </SortableTableHeader>
            <SortableTableHeader column="age" {...trashSort.headerProps} className={`${tableHeadCell} w-28 whitespace-nowrap`}>
              {t("keys.colAge")}
            </SortableTableHeader>
            <SortableTableHeader column="status" {...trashSort.headerProps} className={`${tableHeadCell} w-36 whitespace-nowrap`}>
              {t("keys.colStatus")}
            </SortableTableHeader>
            <th scope="col" className={`${tableHeadCell} w-44`}>
              <span className="sr-only">{t("keys.colActions")}</span>
            </th>
          </tr>
        </thead>
        <tbody className="block @xl/keytable:table-row-group">
          {trashSort.sorted(trash.entries, trashSortValue).map((entry) => (
            <tr key={entry.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-line py-2.5 pl-1 last:border-b-0 @xl/keytable:table-row @xl/keytable:border-b-0">
              <td className={`w-full font-mono text-xs text-ink [overflow-wrap:anywhere] @xl/keytable:w-auto ${cell}`}>
                {trashFilesText(entry)}
              </td>
              <td className={`whitespace-nowrap text-xs text-ink-muted ${cell}`}>
                {entry.stale
                  ? t("keys.ageStale", { days: entry.ageDays, retention: trash.retentionDays })
                  : t("keys.age", { days: entry.ageDays })}
              </td>
              <td className={`text-xs text-ink-muted ${cell}`}>{trashStatusText(entry, t)}</td>
              <td className={`ml-auto text-right ${cell}`}>
                <span className="inline-flex items-center gap-1">
                  <Button small onClick={() => void onRestore(entry.id)}>{t("keys.restore")}</Button>
                  <Button kind="dangerText" onClick={() => setPendingPurge(entry.id)}>{t("keys.purge")}</Button>
                </span>
              </td>
            </tr>
          ))}
          {trash.entries.length === 0 && (
            <tr className="block @xl/keytable:table-row">
              <td colSpan={4} className="block p-8 text-center text-sm text-ink-muted @xl/keytable:table-cell">
                {t("keys.trashEmpty")}
              </td>
            </tr>
          )}
        </tbody>
      </table>
      {pendingPurge === "" ? null : (
        <ConfirmDialog
          id="key-purge-heading"
          heading={t("keys.purge")}
          body={<p className="text-sm text-danger">{t("keys.purgeWarning")}</p>}
          confirmLabel={t("keys.confirmPurge")}
          cancelLabel={t("keys.cancel")}
          onConfirm={async () => {
            setPurgeError("");
            if (await onPurge(pendingPurge)) closePurge();
            else setPurgeError(t("keys.purgeFailed"));
          }}
          onCancel={closePurge}
          error={purgeError}
        />
      )}
    </>
  );
}

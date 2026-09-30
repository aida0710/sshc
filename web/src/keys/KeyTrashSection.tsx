import { useState } from "react";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import type { TrashListResponse } from "./api";
import { useTranslate, type Translate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sectionHeading, tableHeadCell, tableHeadRow } from "../ui/form";
import { SortableTableHeader } from "../ui/tableSort";
import { useTableSort, type SortValue } from "../ui/useTableSort";
import { describeBlockers, rowAction, rowDanger } from "./labels";

type TrashSort = "files" | "age" | "status";
type TrashEntry = TrashListResponse["entries"][number];

function trashFilesText(entry: TrashEntry): string {
  return entry.files.map((file) => file.originalRelativePath).join(", ");
}

function trashStatusText(entry: TrashEntry, t: Translate): string {
  return entry.restorable ? t("keys.restorable") : describeBlockers(entry.blockers, t);
}

// Keys moved to the trash: restorable ones come back whole, the rest wait
// out the retention or are purged after an explicit confirmation.
export function KeyTrashSection({ trash, onRestore, onPurge }: {
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
  return (
    <>
          <details className="border-t border-line bg-surface-subtle p-4">
            <DisclosureSummary className="text-sm font-medium text-ink">
              {t("keys.trashSummary", { count: trash.entries.length })}
            </DisclosureSummary>
            <div className="mt-3 flex flex-col gap-2 border-t border-line pt-3">
              <h3 className={sectionHeading}>{t("keys.trashHeading")}</h3>
              <p className="text-sm text-ink-muted">{t("keys.trashNote")}</p>
              <div className="overflow-x-auto">
                <table className="w-full min-w-[40rem] text-left text-sm">
                  <caption className="sr-only">
                    {t("keys.trashCaption")}
                  </caption>
                  <thead>
                    <tr className={tableHeadRow}>
                      <SortableTableHeader column="files" {...trashSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                        {t("keys.colFiles")}
                      </SortableTableHeader>
                      <SortableTableHeader column="age" {...trashSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                        {t("keys.colAge")}
                      </SortableTableHeader>
                      <SortableTableHeader column="status" {...trashSort.headerProps} className={`${tableHeadCell} whitespace-nowrap`}>
                        {t("keys.colStatus")}
                      </SortableTableHeader>
                      <th
                        scope="col"
                        className={`${tableHeadCell} whitespace-nowrap`}
                      >
                        {t("keys.colActions")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {trashSort.sorted(trash.entries, trashSortValue).map((entry) => (
                      <tr
                        key={entry.id}
                        className="border-b border-line align-top"
                      >
                        <td className="py-2 pr-3 font-mono text-xs">
                          {trashFilesText(entry)}
                        </td>
                        <td className="py-2 pr-3">
                          {entry.stale
                            ? t("keys.ageStale", {
                                days: entry.ageDays,
                                retention: trash.retentionDays,
                              })
                            : t("keys.age", { days: entry.ageDays })}
                        </td>
                        <td className="py-2 pr-3">{trashStatusText(entry, t)}</td>
                        <td className="py-2">
                          <div className="flex flex-wrap items-center gap-1">
                            <button
                              type="button"
                              className={rowAction}
                              onClick={() => void onRestore(entry.id)}
                            >
                              {t("keys.restore")}
                            </button>
                            <button
                              type="button"
                              className={rowDanger}
                              onClick={() => setPendingPurge(entry.id)}
                            >
                              {t("keys.purge")}
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                    {trash.entries.length === 0 && (
                      <tr>
                        <td colSpan={4} className="py-3 text-sm text-ink-muted">
                          {t("keys.trashEmpty")}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </details>
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

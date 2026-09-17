import { useState } from "react";
import type { TrashListResponse } from "./api";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { sectionHeading, tableHeadCell, tableHeadRow } from "../ui/form";
import { SortableTableHeader, compareText, nextSort, ordered, type SortDirection } from "../ui/tableSort";
import { rowAction, rowDanger } from "./labels";

type TrashSort = "files" | "age" | "status";

// Keys moved to the trash: restorable ones come back whole, the rest wait
// out the retention or are purged after an explicit confirmation.
export function KeyTrashSection({ trash, onRestore, onPurge }: {
  trash: TrashListResponse;
  onRestore: (entryId: string) => Promise<void>;
  // Resolves once the entry is gone; the confirmation closes on its own.
  onPurge: (entryId: string) => Promise<boolean>;
}) {
  const t = useTranslate();
  const [trashSort, setTrashSort] = useState<{ key: TrashSort; direction: SortDirection }>({ key: "files", direction: "ascending" });
  const [pendingPurge, setPendingPurge] = useState("");
  return (
    <>
          <details className="border-t border-line bg-surface-subtle p-4">
            <summary className="cursor-pointer text-sm font-medium text-ink">
              {t("keys.trashSummary", { count: trash.entries.length })}
            </summary>
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
                      <SortableTableHeader
                        column="files"
                        activeColumn={trashSort.key}
                        direction={trashSort.direction}
                        onSort={(key) =>
                          setTrashSort((current) =>
                            nextSort(current.key, current.direction, key),
                          )
                        }
                        className={`${tableHeadCell} whitespace-nowrap`}
                      >
                        {t("keys.colFiles")}
                      </SortableTableHeader>
                      <SortableTableHeader
                        column="age"
                        activeColumn={trashSort.key}
                        direction={trashSort.direction}
                        onSort={(key) =>
                          setTrashSort((current) =>
                            nextSort(current.key, current.direction, key),
                          )
                        }
                        className={`${tableHeadCell} whitespace-nowrap`}
                      >
                        {t("keys.colAge")}
                      </SortableTableHeader>
                      <SortableTableHeader
                        column="status"
                        activeColumn={trashSort.key}
                        direction={trashSort.direction}
                        onSort={(key) =>
                          setTrashSort((current) =>
                            nextSort(current.key, current.direction, key),
                          )
                        }
                        className={`${tableHeadCell} whitespace-nowrap`}
                      >
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
                    {ordered(
                      trash.entries,
                      (left, right) => {
                        if (trashSort.key === "age")
                          return left.ageDays - right.ageDays;
                        if (trashSort.key === "status")
                          return compareText(
                            left.restorable
                              ? t("keys.restorable")
                              : left.blockers.join(", "),
                            right.restorable
                              ? t("keys.restorable")
                              : right.blockers.join(", "),
                          );
                        return compareText(
                          left.files
                            .map((file) => file.originalRelativePath)
                            .join(", "),
                          right.files
                            .map((file) => file.originalRelativePath)
                            .join(", "),
                        );
                      },
                      trashSort.direction,
                    ).map((entry) => (
                      <tr
                        key={entry.id}
                        className="border-b border-line align-top"
                      >
                        <td className="py-2 pr-3 font-mono text-xs">
                          {entry.files
                            .map((file) => file.originalRelativePath)
                            .join(", ")}
                        </td>
                        <td className="py-2 pr-3">
                          {entry.stale
                            ? t("keys.ageStale", {
                                days: entry.ageDays,
                                retention: trash.retentionDays,
                              })
                            : t("keys.age", { days: entry.ageDays })}
                        </td>
                        <td className="py-2 pr-3">
                          {entry.restorable
                            ? t("keys.restorable")
                            : entry.blockers.join(", ")}
                        </td>
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
          onConfirm={() => void onPurge(pendingPurge).then((purged) => { if (purged) setPendingPurge(""); })}
          onCancel={() => setPendingPurge("")}
        />
      )}
    </>
  );
}

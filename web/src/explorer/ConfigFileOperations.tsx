import { useTranslate } from "../i18n/context";
import { control, hintText, sectionHeading } from "../ui/form";
import { Button } from "../ui/surface";

// ConfigFileOperations は、開いているファイルの名前を変える・消す欄である。
// 保存していない変更があるあいだは、どちらもできない。
export function ConfigFileOperations({
  path,
  modified,
  renameTo,
  onRenameToChange,
  onRename,
  onDelete,
}: {
  path: string;
  modified: boolean;
  renameTo: string;
  onRenameToChange: (path: string) => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-col gap-2 border-t border-line bg-surface-subtle p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h4 className={sectionHeading}>{t("explorer.fileOperations")}</h4>
          <p className={`mt-1 ${hintText}`}>{t("explorer.fileOperationsNote")}</p>
        </div>
        <div className="flex min-w-0 flex-1 flex-wrap items-end justify-end gap-2 sm:flex-nowrap">
          <label htmlFor="rename-file-path" className="sr-only">{t("explorer.renameTo")}</label>
          <input
            id="rename-file-path"
            value={renameTo}
            onChange={(event) => onRenameToChange(event.target.value)}
            placeholder={path}
            className={`${control} min-h-10 min-w-48 max-w-sm font-mono text-xs md:min-h-0`}
          />
          <Button
            className="min-h-10 md:min-h-0"
            onClick={onRename}
            disabled={renameTo === "" || renameTo === path || modified}
          >
            {t("explorer.renameFile")}
          </Button>
          <Button className="min-h-10 md:min-h-0" onClick={onDelete} disabled={modified}>
            {t("explorer.deleteFile")}
          </Button>
        </div>
      </div>
      <p className={hintText}>{modified ? t("explorer.saveOrDiscardFirst") : t("explorer.deleteIsRecoverable")}</p>
    </div>
  );
}

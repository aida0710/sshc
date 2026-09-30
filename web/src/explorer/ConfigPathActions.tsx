import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { control, fieldLabel, hintText, sectionHeading } from "../ui/form";
import { Button } from "../ui/surface";

// ConfigPathActions は、~/.ssh の中にファイルやフォルダを作ったり、フォルダを
// 消したりする欄である。どの操作も入力したパスを使う。
export function ConfigPathActions({
  path,
  onPathChange,
  onCreateFile,
  onCreateDirectory,
  onDeleteDirectory,
}: {
  path: string;
  onPathChange: (path: string) => void;
  onCreateFile: () => void;
  onCreateDirectory: () => void;
  onDeleteDirectory: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="flex flex-col gap-2 border-t border-line bg-toolbar p-3">
      <h3 className={sectionHeading}>{t("explorer.sshDirectoryFiles")}</h3>
      <label htmlFor="new-file-path" className={fieldLabel}>{t("explorer.newFilePath")}</label>
      <input
        id="new-file-path"
        value={path}
        onChange={(event) => onPathChange(event.target.value)}
        placeholder="conf.d/30-lab.conf"
        className={`${control} min-h-10 font-mono text-xs md:min-h-0`}
      />
      <div className="flex flex-wrap gap-2">
        <Button className="min-h-10 md:min-h-0" onClick={onCreateFile} disabled={path === ""}>
          {t("explorer.createFile")}
        </Button>
        <Button className="min-h-10 md:min-h-0" onClick={onCreateDirectory} disabled={path === ""}>
          {t("explorer.createDirectory")}
        </Button>
        <Button kind="danger" className="min-h-10 md:min-h-0" onClick={onDeleteDirectory} disabled={path === ""}>
          {t("explorer.deleteDirectory")}
        </Button>
      </div>
      <p className={hintText}>{t("explorer.newFileNote")}</p>
      <details className="text-xs text-ink-muted">
        <DisclosureSummary className="text-ink">{t("explorer.directoryHelp")}</DisclosureSummary>
        <p className="mt-2 leading-5">{t("explorer.directoryNote")}</p>
      </details>
    </div>
  );
}

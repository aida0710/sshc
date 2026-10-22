import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "../ui/DisclosureSummary";
import { control, fieldLabel, hintText, sectionHeading } from "../ui/form";
import { Button } from "../ui/surface";
import type { ConfigOperation } from "./configOperation";

// ConfigPathActions は、~/.ssh の中にファイルやフォルダを作ったり、フォルダを
// 消したりする欄である。どの操作も入力したパスを使う。
export function ConfigPathActions({
  path,
  problem,
  pendingOperation,
  onPathChange,
  onCreateFile,
  onCreateDirectory,
  onDeleteDirectory,
}: {
  path: string;
  // problem は、入力したパスが ~/.ssh の外を指しているときに欄の下へ出す理由である。
  problem: string;
  pendingOperation: ConfigOperation | null;
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
        aria-invalid={problem === "" ? undefined : true}
        aria-describedby={problem === "" ? undefined : "new-file-path-problem"}
        className={`${control} min-h-10 font-mono text-xs md:min-h-0`}
      />
      {problem === "" ? null : <p id="new-file-path-problem" role="alert" className="text-xs text-danger">{problem}</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          className="min-h-10 md:min-h-0"
          onClick={onCreateFile}
          busy={pendingOperation === "createFile"}
          disabled={path === "" || pendingOperation !== null}
        >
          {t("explorer.createFile")}
        </Button>
        <Button
          className="min-h-10 md:min-h-0"
          onClick={onCreateDirectory}
          busy={pendingOperation === "createDirectory"}
          disabled={path === "" || pendingOperation !== null}
        >
          {t("explorer.createDirectory")}
        </Button>
        <Button
          kind="danger"
          className="min-h-10 md:min-h-0"
          onClick={onDeleteDirectory}
          busy={pendingOperation === "deleteDirectory"}
          disabled={path === "" || pendingOperation !== null}
        >
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

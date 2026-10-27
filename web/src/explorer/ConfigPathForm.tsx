import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { Button } from "../ui/surface";

// ConfigPathTask は、~/.ssh の中のパスを1つ入れて行う操作である。［ファイルを作成］と「…」のメニューから始める。
export type ConfigPathTask = "createFile" | "createDirectory" | "deleteDirectory" | "rename";

const taskTexts: Record<ConfigPathTask, { label: MessageKey; submit: MessageKey; note: MessageKey }> = {
  createFile: { label: "explorer.newFilePath", submit: "explorer.submitCreate", note: "explorer.newFileNote" },
  createDirectory: { label: "explorer.directoryPath", submit: "explorer.submitCreate", note: "explorer.createDirectoryNote" },
  deleteDirectory: { label: "explorer.directoryPath", submit: "explorer.submitDelete", note: "explorer.deleteDirectoryNote" },
  rename: { label: "explorer.renameTo", submit: "explorer.submitRename", note: "explorer.renameNote" },
};

// ConfigPathForm は、開いているファイルの見出しの下に開く、パスを入れる欄である。入力のために画面を
// ふさがないよう、ダイアログにはしない。
export function ConfigPathForm({
  task,
  path,
  currentPath,
  problem,
  submitting,
  busy,
  onPathChange,
  onSubmit,
  onCancel,
}: {
  task: ConfigPathTask;
  path: string;
  // currentPath は、名前を変えるファイルのいまのパスである。欄の薄い字に出し、同じパスへは変えさせない。
  currentPath: string;
  // problem は、入力したパスが ~/.ssh の外を指しているときに欄の下へ出す理由である。
  problem: string;
  submitting: boolean;
  // busy は、SSH Config のどれかの操作を待っている間である。終わるまでボタンを止める。
  busy: boolean;
  onPathChange: (path: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  const texts = taskTexts[task];
  const unchanged = path.trim() === "" || (task === "rename" && path.trim() === currentPath);
  return (
    <form
      className="mb-3 flex flex-col gap-2 rounded-[10px] bg-well px-3.5 py-3"
      onSubmit={(event) => {
        event.preventDefault();
        if (!unchanged && !busy) onSubmit();
      }}
    >
      <label htmlFor="config-path" className="text-[13px] font-medium text-ink">{t(texts.label)}</label>
      <div className="flex flex-wrap gap-2">
        <input
          id="config-path"
          autoFocus
          value={path}
          onChange={(event) => onPathChange(event.target.value)}
          placeholder={task === "rename" ? currentPath : "conf.d/30-lab.conf"}
          aria-invalid={problem === "" ? undefined : true}
          aria-describedby={problem === "" ? "config-path-note" : "config-path-problem config-path-note"}
          className={`${control} min-h-10 basis-full font-mono text-xs sm:min-w-48 sm:flex-1 sm:basis-0 md:min-h-0`}
        />
        <Button type="submit" kind={task === "deleteDirectory" ? "danger" : "primary"} className="min-h-10 md:min-h-0" busy={submitting} disabled={unchanged || busy}>
          {t(texts.submit)}
        </Button>
        <Button className="min-h-10 md:min-h-0" onClick={onCancel}>{t("explorer.cancelPathTask")}</Button>
      </div>
      {problem === "" ? null : <p id="config-path-problem" role="alert" className="text-xs text-danger">{problem}</p>}
      <p id="config-path-note" className="text-xs leading-[18px] text-ink-faint">{t(texts.note)}</p>
    </form>
  );
}

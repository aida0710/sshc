import { tagClass } from "../ui/Tag";
import type { FileContents } from "../api/config";
import { useTranslate } from "../i18n/context";
import { ActionMenu, type ActionMenuItem } from "../ui/ActionMenu";
import { Button } from "../ui/surface";

// ConfigFileHead は、開いているファイルの見出しの行である。状態の点・名前・編集できるかの札と、
// ファイルとディレクトリの操作の「…」、プレビューのボタンを並べる。
export function ConfigFileHead({
  file,
  modified,
  actions,
  previewing,
  busy,
  onPreview,
}: {
  file: FileContents;
  modified: boolean;
  actions: readonly ActionMenuItem[];
  previewing: boolean;
  // busy は、SSH Config のどれかの操作を待っている間である。終わるまでボタンを止める。
  busy: boolean;
  onPreview: () => void;
}) {
  const t = useTranslate();
  const name = file.file.path ?? file.file.absolute;
  return (
    <div className="mb-3 flex flex-wrap items-center gap-x-2.5 gap-y-2">
      {/* The name keeps at least 12rem, so on a phone the buttons go to a line of their own instead of hiding it. */}
      <div className="flex min-w-48 flex-1 items-center gap-2.5">
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${modified ? "bg-notice-ink" : file.editable ? "bg-live" : "bg-ink-faint"}`} />
        <h3 title={name} className="min-w-0 truncate font-mono text-sm font-semibold text-ink">{name}</h3>
        <span className={tagClass("neutral")}>
          {file.editable ? t("explorer.editable") : t("explorer.readOnly")}
        </span>
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-2">
        <ActionMenu
          label={t("explorer.fileActions", { path: name })}
          items={actions}
          triggerSize="panel"
        />
        <Button className="min-h-10 md:min-h-0" busy={previewing} disabled={busy} onClick={onPreview}>
          {t("explorer.preview")}
        </Button>
      </div>
    </div>
  );
}

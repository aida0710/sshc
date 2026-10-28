import { useId, useState } from "react";
import type { FileNode, GroupMetadata, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import { control, hintText } from "../ui/form";
import { Button } from "../ui/surface";
import { usePendingAction } from "../ui/usePendingAction";

// ConnectionAction は、見出しの「…」から開いて、入力してから書き込む操作である。複製と削除は入力が要らないので
// ここには無い（複製はすぐ行い、削除は ConfirmDialog で確かめる）。
export type ConnectionAction = "rename" | "group" | "comment" | "file";

type ConnectionActionPanelProps = {
  action: ConnectionAction;
  detail: HostDetail;
  groups: GroupMetadata[];
  files: FileNode[];
  disabled: boolean;
  // どれも保存し終えるまで待てる Promise を返し、保存できたかを true で知らせる。
  onRename: (alias: string) => Promise<boolean>;
  onMoveToGroup: (group: string) => Promise<boolean>;
  onComment: (comment: string) => Promise<boolean>;
  onMoveToFile: (path: string) => Promise<boolean>;
  onClose: () => void;
};

// ConnectionActionPanel は、見出しのすぐ下に開く1つの操作の欄である。書き込めたら閉じる。
// タブの下書きとは別に、その場で書き込む。
export function ConnectionActionPanel({
  action,
  detail,
  groups,
  files,
  disabled,
  onRename,
  onMoveToGroup,
  onComment,
  onMoveToFile,
  onClose,
}: ConnectionActionPanelProps) {
  const t = useTranslate();
  const identity = detail.form.entry.identity;
  const currentGroup = detail.form.entry.group ?? "";
  const initialComment = detail.form.comment;
  const [renameTo, setRenameTo] = useState(identity.alias);
  const [group, setGroup] = useState(currentGroup);
  const [comment, setComment] = useState(initialComment);
  const [file, setFile] = useState("");
  const pending = usePendingAction<ConnectionAction>();
  // 欄の見出しが、その1つの入力欄の名前になる。
  const fieldId = useId();

  function submit(work: () => Promise<boolean>) {
    void pending.start(action, async () => {
      if (await work()) onClose();
    });
  }

  const form = {
    rename: {
      title: t("host.renameAlias"),
      ready: renameTo.trim() !== "" && renameTo !== identity.alias,
      submitLabel: t("host.rename"),
      run: () => onRename(renameTo.trim()),
      body: <input id={fieldId} value={renameTo} onChange={(event) => setRenameTo(event.target.value)} className={`${control} font-mono`} />,
      hint: "",
    },
    group: {
      title: t("host.primaryGroup"),
      ready: group !== currentGroup,
      submitLabel: t("host.moveToGroup"),
      run: () => onMoveToGroup(group),
      body: (
        <select id={fieldId} value={group} onChange={(event) => setGroup(event.target.value)} className={control}>
          <option value="">{t("host.groupNone")}</option>
          {groups.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
        </select>
      ),
      hint: t("host.groupNoneMeans"),
    },
    comment: {
      title: t("host.comment"),
      ready: comment !== initialComment,
      submitLabel: t("host.saveComment"),
      run: () => onComment(comment),
      body: <textarea id={fieldId} value={comment} onChange={(event) => setComment(event.target.value)} rows={3} className={control} />,
      hint: t("host.commentNote"),
    },
    file: {
      title: t("conn.moveToFile"),
      ready: file !== "",
      submitLabel: t("conn.move"),
      run: () => onMoveToFile(file),
      body: (
        <select id={fieldId} value={file} onChange={(event) => setFile(event.target.value)} className={control}>
          <option value="">{t("conn.moveToFilePlaceholder")}</option>
          {files
            .filter((node) => node.editable && node.file.path !== undefined && node.file.path !== identity.path)
            .map((node) => <option key={node.file.absolute} value={node.file.path}>{node.file.path}</option>)}
        </select>
      ),
      hint: t("conn.storageFileNote"),
    },
  }[action];

  return (
    <section className="flex shrink-0 flex-col gap-2 rounded-[10px] bg-surface-subtle p-3">
      <label htmlFor={fieldId} className="text-[13px] font-medium text-ink">{form.title}</label>
      <fieldset disabled={disabled || pending.running !== null} className="contents">
        {form.body}
        {form.hint === "" ? null : <p className={hintText}>{form.hint}</p>}
        {disabled ? <p className="text-xs text-notice-ink">{t("conn.manageDraftBlocked")}</p> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>{t("conn.actionCancel")}</Button>
          <Button kind="primary" disabled={!form.ready} busy={pending.running === action} onClick={() => submit(form.run)}>{form.submitLabel}</Button>
        </div>
      </fieldset>
    </section>
  );
}

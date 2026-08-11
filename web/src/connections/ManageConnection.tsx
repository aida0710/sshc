import { useEffect, useState } from "react";
import type { FileNode, GroupMetadata, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import { control, hintText, sectionHeading } from "../ui/form";
import { Button, Card, Row } from "../ui/surface";

type ManageConnectionProps = {
  detail: HostDetail;
  groups: GroupMetadata[];
  files: FileNode[];
  disabled: boolean;
  onRename: (alias: string) => void;
  onMoveToGroup: (group: string) => void;
  onComment: (comment: string) => void;
  onDuplicate: () => void;
  onMoveToFile: (path: string) => void;
  onDelete: () => void;
};

export function ManageConnection({
  detail,
  groups,
  files,
  disabled,
  onRename,
  onMoveToGroup,
  onComment,
  onDuplicate,
  onMoveToFile,
  onDelete,
}: ManageConnectionProps) {
  const t = useTranslate();
  const identity = detail.form.entry.identity;
  const currentGroup = detail.form.entry.group ?? "";
  const initialComment = detail.form.comment || detail.metadata.note || "";
  const resetKey = `${identity.path}\u0000${identity.alias}\u0000${detail.file.contents}`;
  const [renameTo, setRenameTo] = useState(identity.alias);
  const [group, setGroup] = useState(currentGroup);
  const [comment, setComment] = useState(initialComment);
  const [file, setFile] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  useEffect(() => {
    setRenameTo(identity.alias);
    setGroup(currentGroup);
    setComment(initialComment);
    setFile("");
    setConfirmingDelete(false);
    // resetKey is the committed snapshot these independent controls describe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const renameDirty = renameTo !== "" && renameTo !== identity.alias;
  const commentDirty = comment !== initialComment;
  const groupDirty = group !== currentGroup;

  return (
    <section
      aria-label={t("conn.manageLabel")}
      aria-disabled={disabled}
      className="flex flex-col gap-4 rounded-xl border border-line bg-card p-4"
    >
      <div>
        <h3 className={sectionHeading}>{t("conn.manageLabel")}</h3>
        <p className={hintText}>{t("conn.manageIndependent")}</p>
        {disabled ? <p className="mt-1 text-xs text-notice-ink">{t("conn.manageDraftBlocked")}</p> : null}
      </div>

      <fieldset disabled={disabled} className="contents">
        <Card>
          <Row
            label={t("host.renameAlias")}
            action={<Button disabled={!renameDirty} onClick={() => onRename(renameTo)}>{t("host.rename")}</Button>}
          >
            <input
              aria-label={t("host.renameAlias")}
              value={renameTo}
              onChange={(event) => setRenameTo(event.target.value)}
              className={control}
            />
          </Row>
          <Row
            label={t("host.primaryGroup")}
            hint={t("host.groupNoneMeans")}
            action={<Button disabled={!groupDirty} onClick={() => onMoveToGroup(group)}>{t("host.moveToGroup")}</Button>}
          >
            <select
              aria-label={t("host.primaryGroup")}
              value={group}
              onChange={(event) => setGroup(event.target.value)}
              className={control}
            >
              <option value="">{t("host.groupNone")}</option>
              {groups.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
            </select>
          </Row>
        </Card>

        <label className="flex flex-col gap-2">
          <span className="text-sm text-ink-muted">{t("host.comment")}</span>
          <textarea
            aria-label={t("host.comment")}
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            rows={3}
            className={control}
          />
          <span className={hintText}>
            {detail.form.comment === "" && (detail.metadata.note ?? "") !== ""
              ? t("host.commentFromNote")
              : t("host.commentNote")}
          </span>
        </label>
        <Button className="self-start" disabled={!commentDirty} onClick={() => onComment(comment)}>
          {t("host.saveComment")}
        </Button>

        <div className="flex flex-col gap-3 border-t border-line pt-4">
          <Button className="self-start" onClick={onDuplicate}>{t("conn.duplicate")}</Button>
          <p className={hintText}>{t("conn.storageFileNote")}</p>
          <div className="flex flex-wrap items-center gap-2">
            <label htmlFor="manage-storage-file" className="text-sm text-ink-muted">{t("conn.moveToFile")}</label>
            <select
              id="manage-storage-file"
              aria-label={t("conn.moveToFile")}
              value={file}
              onChange={(event) => setFile(event.target.value)}
              className={control}
            >
              <option value="">{t("conn.moveToFilePlaceholder")}</option>
              {files
                .filter((node) => node.editable && node.file.path !== undefined && node.file.path !== identity.path)
                .map((node) => <option key={node.file.absolute} value={node.file.path}>{node.file.path}</option>)}
            </select>
            <Button disabled={file === ""} onClick={() => onMoveToFile(file)}>{t("conn.move")}</Button>
          </div>
        </div>

        <div className="border-t border-danger/30 pt-4">
          {confirmingDelete ? (
            <Button kind="danger" onClick={onDelete}>{t("conn.confirmDelete")}</Button>
          ) : (
            <Button kind="danger" onClick={() => setConfirmingDelete(true)}>{t("conn.delete")}</Button>
          )}
        </div>
      </fieldset>
    </section>
  );
}

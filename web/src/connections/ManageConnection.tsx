import { useEffect, useState } from "react";
import type { FileNode, GroupMetadata, HostDetail } from "../api/config";
import { useTranslate } from "../i18n/context";
import { control, hintText, sectionHeading } from "../ui/form";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import { identityKey } from "./connectionBrowser";

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
  const resetKey = `${identityKey(identity)}\u0000${detail.file.contents}`;
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const renameDirty = renameTo !== "" && renameTo !== identity.alias;
  const commentDirty = comment !== initialComment;
  const groupDirty = group !== currentGroup;

  return (
    <section
      aria-label={t("conn.manageLabel")}
      aria-disabled={disabled}
      className="sshc-card flex shrink-0 flex-col gap-5 overflow-hidden rounded-md bg-card p-5"
    >
      <div>
        <h3 className="text-base font-semibold tracking-tight text-ink">{t("conn.manageLabel")}</h3>
        <p className={`mt-1 ${hintText}`}>{t("conn.manageIndependent")}</p>
        {disabled ? <p className="mt-1 text-xs text-notice-ink">{t("conn.manageDraftBlocked")}</p> : null}
      </div>

      <fieldset disabled={disabled} className="contents">
        <div className="grid gap-5 border-t border-line pt-5 lg:grid-cols-2">
          <div className="flex min-w-0 flex-col gap-1.5">
            <label htmlFor="manage-rename-alias" className={sectionHeading}>{t("host.renameAlias")}</label>
            <div className="flex items-center gap-2">
              <input
              id="manage-rename-alias"
              aria-label={t("host.renameAlias")}
              value={renameTo}
              onChange={(event) => setRenameTo(event.target.value)}
              className={control}
            />
              <Button disabled={!renameDirty} onClick={() => onRename(renameTo)}>{t("host.rename")}</Button>
            </div>
          </div>
          <div className="flex min-w-0 flex-col gap-1.5">
            <label htmlFor="manage-primary-group" className={sectionHeading}>{t("host.primaryGroup")}</label>
            <div className="flex items-center gap-2">
              <select
              id="manage-primary-group"
              aria-label={t("host.primaryGroup")}
              value={group}
              onChange={(event) => setGroup(event.target.value)}
              className={control}
            >
              <option value="">{t("host.groupNone")}</option>
              {groups.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
            </select>
              <Button disabled={!groupDirty} onClick={() => onMoveToGroup(group)}>{t("host.moveToGroup")}</Button>
            </div>
            <span className={hintText}>{t("host.groupNoneMeans")}</span>
          </div>
        </div>

        <label className="flex flex-col gap-2 border-t border-line pt-5">
          <span className={sectionHeading}>{t("host.comment")}</span>
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

        <div className="grid gap-4 border-t border-line pt-5">
          <div className="flex flex-col gap-2">
            <h4 className={sectionHeading}>{t("conn.moveToFile")}</h4>
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
          <Button className="self-start" onClick={onDuplicate}>{t("conn.duplicate")}</Button>
        </div>

        <div className="border-t border-danger/30 pt-5">
          <Button kind="danger" onClick={() => setConfirmingDelete(true)}>{t("conn.delete")}</Button>
          {confirmingDelete ? (
            <ConfirmDialog
              id="delete-connection-heading"
              heading={t("conn.deleteHeading", { alias: identity.alias })}
              body={<p className="text-sm text-ink-muted">{t("conn.deleteBody")}</p>}
              confirmLabel={t("conn.confirmDelete")}
              cancelLabel={t("conn.deleteCancel")}
              onCancel={() => setConfirmingDelete(false)}
              onConfirm={() => {
                setConfirmingDelete(false);
                onDelete();
              }}
            />
          ) : null}
        </div>
      </fieldset>
    </section>
  );
}

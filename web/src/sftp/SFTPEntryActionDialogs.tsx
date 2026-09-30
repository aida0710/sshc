import { useId, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { InputDialog } from "../ui/InputDialog";
import { entryInputDialogText } from "./entryInputDialogText";
import type { SFTPEntryActionsModel } from "./useSFTPEntryActions";

export function SFTPEntryActionDialogs({ actions, currentPath, returnFocusRef }: {
  actions: SFTPEntryActionsModel;
  currentPath: string;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const id = useId();
  const { inputIntent, deleting } = actions;
  const inputText = inputIntent === null ? null : entryInputDialogText(inputIntent, currentPath);
  return (
    <>
      {deleting === null ? null : (
        <ConfirmDialog
          id={`${id}-delete`}
          heading={deleting.length === 1 ? t("sftp.deleteHeading") : t("sftp.deleteHeadingCount", { count: deleting.length })}
          body={<div className="space-y-2 text-sm text-ink-muted">
            {deleting.some((entry) => entry.type === "directory") ? <p>{t("sftp.deleteContentsWarning")}</p> : null}
            <ul className="max-h-48 space-y-1 overflow-auto">
              {deleting.map((entry) => <li key={entry.path} className="break-all font-mono">{entry.path}</li>)}
            </ul>
          </div>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={returnFocusRef}
          onConfirm={actions.remove}
          onCancel={actions.cancelDelete}
          busy={actions.acting}
          error={actions.deleteProblem}
        />
      )}
      {inputIntent === null || inputText === null ? null : (
        <InputDialog
          id={`${id}-input`}
          heading={t(inputText.heading)}
          label={t(inputText.label)}
          initialValue={inputText.initialValue}
          inputMode={inputIntent.kind === "chmod" ? "numeric" : "text"}
          submitLabel={t(inputText.submit)}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={returnFocusRef}
          validate={(value) => {
            if (inputIntent.kind === "chmod") return /^0?[0-7]{3}$/.test(value) ? "" : t("sftp.chmodInvalid");
            if (inputIntent.kind === "moveTo") return value.startsWith("/") ? "" : t("sftp.pathAbsolute");
            if (value === "") return t("sftp.nameRequired");
            if (value.includes("/")) return t("sftp.nameInvalid");
            if (inputIntent.kind === "rename" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            if (inputIntent.kind === "duplicate" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            return "";
          }}
          onSubmit={actions.submit}
          onCancel={actions.cancelInput}
        />
      )}
    </>
  );
}

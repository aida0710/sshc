import { useTranslate } from "../../i18n/context";
import { ConfirmDialog } from "../../ui/ConfirmDialog";
import { InputDialog } from "../../ui/InputDialog";
import type { SavedWorkspace } from "./api";

// The two dialogs of the saved layouts: naming the layout to save, and
// confirming that a saved layout is deleted.

export function SaveWorkspaceDialog({
  initialName,
  onSave,
  onCancel,
}: {
  initialName: string;
  onSave: (name: string) => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  return (
    <InputDialog
      id="workspace-save-heading"
      heading={t("workspace.save")}
      label={t("workspace.namePrompt")}
      initialValue={initialName}
      submitLabel={t("workspace.saveConfirm")}
      cancelLabel={t("workspace.cancel")}
      validate={(value) => value === "" ? t("workspace.nameRequired") : ""}
      onSubmit={onSave}
      onCancel={onCancel}
    />
  );
}

export function DeleteWorkspaceDialog({
  workspace,
  onDelete,
  onCancel,
}: {
  workspace: SavedWorkspace;
  onDelete: () => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  return (
    <ConfirmDialog
      id="workspace-delete-heading"
      heading={t("workspace.deleteHeading", { name: workspace.name })}
      body={<p className="text-sm text-ink-muted">{t("workspace.deleteBody")}</p>}
      confirmLabel={t("workspace.confirmDelete")}
      cancelLabel={t("workspace.cancel")}
      onCancel={onCancel}
      onConfirm={onDelete}
    />
  );
}

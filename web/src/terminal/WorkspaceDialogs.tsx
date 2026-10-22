import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { InputDialog } from "../ui/InputDialog";

// Naming a workspace, for creating one or renaming one. A built-in workspace
// may be given an empty name, which brings back the name the screen gives it.
export function WorkspaceNameDialog({
  heading,
  submitLabel,
  initialName,
  allowEmpty,
  onSubmit,
  onCancel,
}: {
  heading: string;
  submitLabel: string;
  initialName: string;
  allowEmpty: boolean;
  onSubmit: (name: string) => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  return (
    <InputDialog
      id="workspace-name-heading"
      heading={heading}
      label={t("workspace.nameLabel")}
      initialValue={initialName}
      submitLabel={submitLabel}
      cancelLabel={t("workspace.cancel")}
      validate={(value) => value === "" && !allowEmpty ? t("workspace.nameRequired") : ""}
      onSubmit={onSubmit}
      onCancel={onCancel}
    />
  );
}

// Deleting a workspace closes nothing: its sessions move to the default one.
export function DeleteWorkspaceDialog({
  name,
  sessionCount,
  destinationName,
  onDelete,
  onCancel,
}: {
  name: string;
  sessionCount: number;
  // destinationName is the default workspace the sessions move to, as shown.
  destinationName: string;
  onDelete: () => void;
  onCancel: () => void;
}) {
  const t = useTranslate();
  return (
    <ConfirmDialog
      id="workspace-delete-heading"
      heading={t("workspace.deleteHeading", { name })}
      body={
        <p className="text-sm text-ink-muted">
          {sessionCount === 0 ? t("workspace.deleteEmptyBody") : t("workspace.deleteBody", { count: String(sessionCount), destination: destinationName })}
        </p>
      }
      confirmLabel={t("workspace.confirmDelete")}
      cancelLabel={t("workspace.cancel")}
      onCancel={onCancel}
      onConfirm={onDelete}
    />
  );
}

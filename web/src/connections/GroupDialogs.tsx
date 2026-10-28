import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { control, fieldLabel } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";
import { groupSegmentProblem, joinGroup } from "./groupNameRules";

// グループの「…」から開くダイアログ。どれを開いているかを Connections 画面が持つ。
export type GroupDialogState =
  | { kind: "create"; parent: string }
  | { kind: "rename"; name: string }
  | { kind: "delete"; name: string; memberCount: number };

// 作成・名前の変更・削除は、Promise を返すとダイアログを開いたまま待つ。断られた理由は error で中に出す。
type GroupDialogsProps = {
  dialog: GroupDialogState;
  groupNames: readonly string[];
  onCreate: (name: string) => void | Promise<unknown>;
  onRename: (from: string, to: string) => void | Promise<unknown>;
  onDelete: (name: string, destination: string) => void | Promise<unknown>;
  onClose: () => void;
  error?: string;
};

function lastSegment(name: string): string {
  return name.slice(name.lastIndexOf("/") + 1);
}

function parentOf(name: string): string {
  const index = name.lastIndexOf("/");
  return index < 0 ? "" : name.slice(0, index);
}

export function GroupDialogs({ dialog, groupNames, onCreate, onRename, onDelete, onClose, error = "" }: GroupDialogsProps) {
  const t = useTranslate();
  const [destination, setDestination] = useState("");
  const taken = new Set(groupNames.map((name) => name.toLowerCase()));

  function validateSegment(parent: string, current: string | null) {
    return (value: string) => groupSegmentProblem(t, { parent, value, current, taken });
  }

  if (dialog.kind === "create") {
    return (
      <InputDialog
        id="group-create-heading"
        heading={dialog.parent === "" ? t("groupDialog.createHeading") : t("groupDialog.createInside", { name: dialog.parent })}
        label={t("groupDialog.nameLabel")}
        submitLabel={t("groupDialog.create")}
        cancelLabel={t("groupDialog.cancel")}
        validate={validateSegment(dialog.parent, null)}
        onSubmit={(value) => onCreate(joinGroup(dialog.parent, value))}
        onCancel={onClose}
        error={error}
      />
    );
  }

  if (dialog.kind === "rename") {
    const parent = parentOf(dialog.name);
    return (
      <InputDialog
        id="group-rename-heading"
        heading={t("groupDialog.renameHeading", { name: dialog.name })}
        label={t("groupDialog.nameLabel")}
        initialValue={lastSegment(dialog.name)}
        submitLabel={t("tree.rename")}
        cancelLabel={t("groupDialog.cancel")}
        validate={validateSegment(parent, dialog.name)}
        onSubmit={(value) => {
          const target = joinGroup(parent, value);
          if (target === dialog.name) {
            onClose();
            return;
          }
          return onRename(dialog.name, target);
        }}
        onCancel={onClose}
        error={error}
      />
    );
  }

  const candidates = groupNames.filter((name) => name !== dialog.name && !name.startsWith(`${dialog.name}/`));
  return (
    <ConfirmDialog
      id="group-delete-heading"
      heading={t("groupDialog.deleteHeading", { name: dialog.name })}
      body={dialog.memberCount === 0 ? (
        <p className="text-sm text-ink-muted">{t("groupDialog.deleteEmpty")}</p>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-ink-muted">{t("groupDialog.deleteKeepsConnections")}</p>
          <label className="flex flex-col gap-1">
            <span className={fieldLabel}>{t("groupDialog.deleteMoveTo", { count: dialog.memberCount })}</span>
            <select value={destination} onChange={(event) => setDestination(event.currentTarget.value)} className={control}>
              <option value="">{t("tree.ungrouped")}</option>
              {candidates.map((name) => <option key={name} value={name}>{name}</option>)}
            </select>
          </label>
        </div>
      )}
      confirmLabel={t("groupDialog.delete")}
      cancelLabel={t("groupDialog.cancel")}
      onConfirm={() => onDelete(dialog.name, destination)}
      onCancel={onClose}
      error={error}
    />
  );
}

import { useState } from "react";
import { useTranslate } from "../i18n/context";
import { maxGroupSegments } from "../rules/generated";
import { isValidGroupName } from "../rules/rules";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { control, fieldLabel } from "../ui/form";
import { InputDialog } from "../ui/InputDialog";

// グループの「…」から開くダイアログ。どれを開いているかを Connections 画面が持つ。
export type GroupDialogState =
  | { kind: "create"; parent: string }
  | { kind: "rename"; name: string }
  | { kind: "delete"; name: string; memberCount: number };

type GroupDialogsProps = {
  dialog: GroupDialogState;
  groupNames: readonly string[];
  onCreate: (name: string) => void;
  onRename: (from: string, to: string) => void;
  onDelete: (name: string, destination: string) => void;
  onClose: () => void;
};

function lastSegment(name: string): string {
  return name.slice(name.lastIndexOf("/") + 1);
}

function parentOf(name: string): string {
  const index = name.lastIndexOf("/");
  return index < 0 ? "" : name.slice(0, index);
}

function joinGroup(parent: string, segment: string): string {
  return parent === "" ? segment : `${parent}/${segment}`;
}

export function GroupDialogs({ dialog, groupNames, onCreate, onRename, onDelete, onClose }: GroupDialogsProps) {
  const t = useTranslate();
  const [destination, setDestination] = useState("");
  const taken = new Set(groupNames.map((name) => name.toLowerCase()));

  // 名前の欄には、親を除いた最後の1段だけを入れてもらう。階層は「中にグループを作成」とドラッグで決める。
  function validateSegment(parent: string, current: string | null) {
    return (value: string) => {
      if (value.includes("/")) return t("groupDialog.noSlash");
      if (!isValidGroupName(value)) return t("groupDialog.invalidCharacters");
      const full = joinGroup(parent, value);
      if (!isValidGroupName(full)) return t("groupDialog.tooDeep", { count: maxGroupSegments });
      if (full !== current && taken.has(full.toLowerCase())) return t("groups.nameTaken");
      return "";
    };
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
          if (target === dialog.name) onClose();
          else onRename(dialog.name, target);
        }}
        onCancel={onClose}
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
    />
  );
}

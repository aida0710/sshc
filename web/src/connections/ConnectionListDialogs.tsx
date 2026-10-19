import type { HostIdentity } from "../api/config";
import { useTranslate } from "../i18n/context";
import { isValidAlias } from "../rules/rules";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { InputDialog } from "../ui/InputDialog";
import { identityKey } from "./connectionBrowser";
import { GroupDialogs } from "./GroupDialogs";
import { MoveToGroupDialog } from "./MoveToGroupDialog";
import type { ConnectionListActions } from "./useConnectionListActions";

type ConnectionListDialogsProps = {
  actions: ConnectionListActions;
  createGroup: (name: string) => Promise<boolean>;
  // グループの名前の変更と削除は、保存できたかを返す。保存できたときだけ絞り込みを追わせる。
  renameGroup: (from: string, to: string) => Promise<boolean>;
  deleteGroup: (name: string, destination: string) => Promise<boolean>;
  renameConnection: (host: HostIdentity, newAlias: string) => Promise<void>;
};

// ConnectionListDialogs は、Connections の一覧の「…」と選択の帯から開くダイアログを並べる。
// どのダイアログを開いているかは useConnectionListActions が持つ。
export function ConnectionListDialogs({ actions, createGroup, renameGroup, deleteGroup, renameConnection }: ConnectionListDialogsProps) {
  const t = useTranslate();
  const { groupDialog, moving, deleting, renamingConnection } = actions;

  return (
    <>
      {groupDialog === null ? null : (
        <GroupDialogs
          dialog={groupDialog}
          groupNames={actions.groupListings.map((group) => group.name)}
          onCreate={(name) => {
            actions.closeGroupDialog();
            void createGroup(name);
          }}
          onRename={(from, to) => {
            actions.closeGroupDialog();
            void renameGroup(from, to).then((saved) => {
              if (saved) actions.followGroupRename(from, to);
            });
          }}
          onDelete={(name, destination) => {
            actions.closeGroupDialog();
            void deleteGroup(name, destination).then((saved) => {
              if (saved) actions.forgetDeletedGroup(name);
            });
          }}
          onClose={actions.closeGroupDialog}
        />
      )}
      {moving === null ? null : (
        <MoveToGroupDialog
          count={moving.length}
          groups={actions.groupListings}
          currentGroup={moving.every((host) => (host.group ?? "") === (moving[0]?.group ?? "")) ? (moving[0]?.group ?? "") : null}
          onPick={(group) => void actions.moveChosenTo(group)}
          onCancel={actions.cancelMove}
        />
      )}
      {deleting === null ? null : (
        <ConfirmDialog
          id="connections-delete-heading"
          heading={deleting.length === 1
            ? t("conn.deleteHeading", { alias: deleting[0]?.identity.alias ?? "" })
            : t("bulkDelete.heading", { count: deleting.length })}
          body={(
            <ul className="flex max-h-48 flex-wrap gap-1.5 overflow-y-auto">
              {deleting.map((host) => (
                <li key={identityKey(host.identity)} className="rounded-md bg-surface-subtle px-2 py-1 font-mono text-xs text-ink">{host.identity.alias}</li>
              ))}
            </ul>
          )}
          confirmLabel={t("conn.confirmDelete")}
          cancelLabel={t("conn.deleteCancel")}
          onConfirm={() => void actions.deleteChosen()}
          onCancel={actions.cancelDelete}
        />
      )}
      {renamingConnection === null ? null : (
        <InputDialog
          id="connection-rename-heading"
          heading={t("connectionDialog.renameHeading", { alias: renamingConnection.identity.alias })}
          label={t("host.renameAlias")}
          initialValue={renamingConnection.identity.alias}
          submitLabel={t("tree.rename")}
          cancelLabel={t("groupDialog.cancel")}
          validate={(value) => (isValidAlias(value) ? "" : t("conn.createAliasInvalid"))}
          onSubmit={(value) => {
            actions.closeConnectionRename();
            if (value !== renamingConnection.identity.alias) void renameConnection(renamingConnection.identity, value);
          }}
          onCancel={actions.closeConnectionRename}
        />
      )}
    </>
  );
}

import { useState } from "react";
import type { HostIdentity } from "../api/config";
import { useTranslate } from "../i18n/context";
import { isValidAlias } from "../rules/rules";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { InputDialog } from "../ui/InputDialog";
import { useToast } from "../ui/useToast";
import { identityKey } from "./connectionBrowser";
import { GroupDialogs } from "./GroupDialogs";
import { movedConnectionsMessage } from "./movedMessage";
import { MoveToGroupDialog } from "./MoveToGroupDialog";
import type { ConnectionListActions } from "./useConnectionListActions";

type ConnectionListDialogsProps = {
  actions: ConnectionListActions;
  // どの操作も、保存できたかを返す。
  createGroup: (name: string) => Promise<boolean>;
  renameGroup: (from: string, to: string) => Promise<boolean>;
  deleteGroup: (name: string, destination: string) => Promise<boolean>;
  renameConnection: (host: HostIdentity, newAlias: string) => Promise<boolean>;
  // 保存を断られた理由。背面の画面に出すとダイアログに隠れるので、開いているダイアログの中にも出す。
  failure: string;
};

// ConnectionListDialogs は、Connections の一覧の「…」と選択の帯から開くダイアログを並べる。
// どのダイアログを開いているかは useConnectionListActions が持つ。どのダイアログも、保存を
// 開いたまま待ち、できたら閉じて知らせ、断られたら理由を中に出す。
export function ConnectionListDialogs({ actions, createGroup, renameGroup, deleteGroup, renameConnection, failure }: ConnectionListDialogsProps) {
  const t = useTranslate();
  const showToast = useToast();
  const { groupDialog, moving, deleting, renamingConnection } = actions;
  // 開く前からあった失敗は出さず、このダイアログで試して断られたときだけ出す。
  const [attemptFailed, setAttemptFailed] = useState(false);
  const dialogError = attemptFailed ? failure : "";

  async function attempt(save: () => Promise<boolean>, { close, done }: { close: () => void; done: string }) {
    setAttemptFailed(false);
    if (!(await save())) {
      setAttemptFailed(true);
      return;
    }
    close();
    showToast(done, { tone: "success" });
  }

  function closeWith(close: () => void) {
    return () => {
      setAttemptFailed(false);
      close();
    };
  }

  return (
    <>
      {groupDialog === null ? null : (
        <GroupDialogs
          dialog={groupDialog}
          groupNames={actions.groupListings.map((group) => group.name)}
          onCreate={(name) => attempt(() => createGroup(name), {
            close: actions.closeGroupDialog,
            done: t("groupDialog.created", { name }),
          })}
          onRename={(from, to) => attempt(async () => {
            const saved = await renameGroup(from, to);
            if (saved) actions.followGroupRename(from, to);
            return saved;
          }, { close: actions.closeGroupDialog, done: t("groupDialog.renamed", { name: to }) })}
          onDelete={(name, destination) => attempt(async () => {
            const saved = await deleteGroup(name, destination);
            if (saved) actions.forgetDeletedGroup(name);
            return saved;
          }, { close: actions.closeGroupDialog, done: t("groupDialog.deleted", { name }) })}
          onClose={closeWith(actions.closeGroupDialog)}
          error={dialogError}
        />
      )}
      {moving === null ? null : (
        <MoveToGroupDialog
          aliases={moving.map((host) => host.identity.alias)}
          groups={actions.groupListings}
          currentGroup={moving.every((host) => (host.group ?? "") === (moving[0]?.group ?? "")) ? (moving[0]?.group ?? "") : null}
          onPick={(group) => attempt(() => actions.moveChosenTo(group), {
            close: actions.closeMove,
            done: movedConnectionsMessage(t, { aliases: moving.map((host) => host.identity.alias), group }),
          })}
          onCancel={closeWith(actions.closeMove)}
          error={dialogError}
        />
      )}
      {deleting === null ? null : (
        <ConfirmDialog
          id="connections-delete-heading"
          heading={deleting.length === 1
            ? t("conn.deleteHeading", { alias: deleting[0]?.identity.alias ?? "" })
            : t("bulkDelete.heading", { count: deleting.length })}
          body={(
            <div className="flex flex-col gap-3">
              <p className="text-sm text-ink-muted">{deleting.length === 1 ? t("conn.deleteBody") : t("bulkDelete.body")}</p>
              <ul className="flex max-h-48 flex-wrap gap-1.5 overflow-y-auto">
                {deleting.map((host) => (
                  <li key={identityKey(host.identity)} className="rounded-md bg-surface-subtle px-2 py-1 font-mono text-xs text-ink">{host.identity.alias}</li>
                ))}
              </ul>
            </div>
          )}
          confirmLabel={deleting.length === 1 ? t("conn.confirmDelete") : t("bulkDelete.confirm", { count: deleting.length })}
          cancelLabel={t("conn.deleteCancel")}
          onConfirm={() => attempt(actions.deleteChosen, {
            close: actions.closeDelete,
            done: deleting.length === 1
              ? t("bulkDelete.deletedOne", { alias: deleting[0]?.identity.alias ?? "" })
              : t("bulkDelete.deleted", { count: deleting.length }),
          })}
          onCancel={closeWith(actions.closeDelete)}
          error={dialogError}
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
            if (value === renamingConnection.identity.alias) {
              actions.closeConnectionRename();
              return;
            }
            return attempt(() => renameConnection(renamingConnection.identity, value), {
              close: actions.closeConnectionRename,
              done: t("connectionDialog.renamed", { alias: value }),
            });
          }}
          onCancel={closeWith(actions.closeConnectionRename)}
          error={dialogError}
        />
      )}
    </>
  );
}

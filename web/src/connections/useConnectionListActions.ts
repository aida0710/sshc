import { useMemo, useState } from "react";
import type { HostEntry, HostIdentity, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { identityKey } from "./connectionBrowser";
import type { HostSelection, Scope } from "./ConnectionTree";
import type { GroupDialogState } from "./GroupDialogs";
import { listGroups } from "./groupListing";
import { emptyMultiSelection, type MultiSelection } from "./multiSelection";

type ConnectionListActionsOptions = {
  overview: Overview | null;
  // 狭い画面では、右側（グループの編集）を開くと一覧が隠れるので、［編集］で開いたときだけ出す。
  compact: boolean;
  selection: HostSelection | null;
  movesDisabled: boolean;
  clearTarget: () => void;
  // グループの絞り込みや編集欄を切り替える前に、保存していない下書きを捨ててよいかを確かめる。
  // connectionDraft が true なら、開いている接続先の下書きも対象にする（接続先を閉じるため）。
  confirmBeforeLeaving: (proceed: () => void, options: { connectionDraft: boolean }) => void;
  onOpenSSHSession: (alias: string) => Promise<void>;
  moveConnections: (hosts: readonly HostIdentity[], group: string) => Promise<void>;
  deleteConnections: (hosts: readonly HostIdentity[]) => Promise<void>;
  duplicateConnection: (host: HostIdentity) => Promise<void>;
};

// useConnectionListActions は、Connections の一覧で選んでいるもの（グループの絞り込み、複数選択）と、
// 一覧の「…」や帯から開くダイアログの状態を持つ。ダイアログで確かめたあとの書き込みは呼び手の関数を使う。
export function useConnectionListActions({
  overview,
  compact,
  selection,
  movesDisabled,
  clearTarget,
  confirmBeforeLeaving,
  onOpenSSHSession,
  moveConnections,
  deleteConnections,
  duplicateConnection,
}: ConnectionListActionsOptions) {
  const t = useTranslate();
  const [scope, setScope] = useState<Scope>({ kind: "all" });
  const [checked, setChecked] = useState<MultiSelection>(emptyMultiSelection);
  const [selectionMode, setSelectionMode] = useState(false);
  // 「…」の［編集］で開いたグループ。広い画面では、グループを選んで接続先を開いていなければそのグループを出す。
  const [editingGroup, setEditingGroup] = useState<string | null>(null);
  const [groupDialog, setGroupDialog] = useState<GroupDialogState | null>(null);
  const [moving, setMoving] = useState<HostEntry[] | null>(null);
  const [deleting, setDeleting] = useState<HostEntry[] | null>(null);
  const [renamingConnection, setRenamingConnection] = useState<HostEntry | null>(null);

  const groupListings = useMemo(() => (overview === null ? [] : listGroups(overview)), [overview]);
  const hostsByKey = useMemo(
    () => new Map((overview?.hosts ?? []).filter((host) => host.identity.alias !== "").map((host) => [identityKey(host.identity), host])),
    [overview],
  );
  const checkedHosts = [...checked.keys].flatMap((key) => {
    const host = hostsByKey.get(key);
    return host === undefined ? [] : [host];
  });
  const groupEditorName = editingGroup ?? (!compact && selection === null && scope.kind === "group" ? scope.name : null);

  function changeScope(next: Scope) {
    confirmBeforeLeaving(() => {
      setScope(next);
      setEditingGroup(null);
    }, { connectionDraft: false });
  }

  // 名前を変えたグループや消したグループを絞り込んでいたら、絞り込みを追従させる。
  function followGroupRename(from: string, to: string) {
    if (scope.kind === "group" && scope.name === from) setScope({ kind: "group", name: to });
  }

  function forgetDeletedGroup(name: string) {
    if (scope.kind === "group" && (scope.name === name || scope.name.startsWith(`${name}/`))) {
      setScope({ kind: "all" });
      setEditingGroup(null);
    }
  }

  function clearChecked() {
    setChecked(emptyMultiSelection);
    setSelectionMode(false);
  }

  function openGroupEditor(name: string) {
    confirmBeforeLeaving(() => {
      setScope({ kind: "group", name });
      setEditingGroup(name);
      clearChecked();
      if (selection !== null) clearTarget();
    }, { connectionDraft: true });
  }

  // groupActionItems は、グループの「…」に並べる操作。グループの編集欄の中の「…」には［編集］を出さない。
  function groupActionItems(name: string, { inEditor = false }: { inEditor?: boolean } = {}): ActionMenuItem[] {
    const memberCount = groupListings.find((group) => group.name === name)?.count ?? 0;
    const edit: ActionMenuItem[] = inEditor ? [] : [{ label: t("tree.editGroup"), onSelect: () => openGroupEditor(name) }];
    return [
      { label: t("tree.newSubgroup"), onSelect: () => setGroupDialog({ kind: "create", parent: name }) },
      { label: t("tree.rename"), onSelect: () => setGroupDialog({ kind: "rename", name }), disabled: movesDisabled },
      ...edit,
      { label: t("tree.deleteEllipsis"), tone: "danger", onSelect: () => setGroupDialog({ kind: "delete", name, memberCount }), disabled: movesDisabled },
    ];
  }

  function connectionActionItems(host: HostEntry): ActionMenuItem[] {
    return [
      { label: t("tree.connect"), onSelect: () => void onOpenSSHSession(host.identity.alias) },
      { label: t("tree.rename"), onSelect: () => setRenamingConnection(host), disabled: movesDisabled },
      { label: t("tree.duplicate"), onSelect: () => void duplicateConnection(host.identity), disabled: movesDisabled },
      { label: t("tree.moveToGroup"), onSelect: () => setMoving([host]), disabled: movesDisabled },
      { label: t("tree.deleteEllipsis"), tone: "danger", onSelect: () => setDeleting([host]), disabled: movesDisabled },
    ];
  }

  async function moveChosenTo(group: string) {
    const hosts = moving ?? [];
    setMoving(null);
    await moveConnections(hosts.map((host) => host.identity), group);
    clearChecked();
  }

  async function deleteChosen() {
    const hosts = deleting ?? [];
    setDeleting(null);
    await deleteConnections(hosts.map((host) => host.identity));
    clearChecked();
  }

  return {
    scope,
    changeScope,
    followGroupRename,
    forgetDeletedGroup,
    checked,
    setChecked,
    checkedHosts,
    clearChecked,
    selectionMode,
    setSelectionMode,
    editingGroup,
    closeGroupEditor: () => setEditingGroup(null),
    groupEditorName,
    groupListings,
    groupActionItems,
    connectionActionItems,
    groupDialog,
    openCreateGroup: (parent: string) => setGroupDialog({ kind: "create", parent }),
    closeGroupDialog: () => setGroupDialog(null),
    moving,
    chooseMoveTarget: (hosts: HostEntry[]) => setMoving(hosts),
    cancelMove: () => setMoving(null),
    moveChosenTo,
    deleting,
    confirmDeleting: (hosts: HostEntry[]) => setDeleting(hosts),
    cancelDelete: () => setDeleting(null),
    deleteChosen,
    renamingConnection,
    closeConnectionRename: () => setRenamingConnection(null),
  };
}

export type ConnectionListActions = ReturnType<typeof useConnectionListActions>;

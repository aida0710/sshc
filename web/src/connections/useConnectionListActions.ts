import { useMemo, useState } from "react";
import type { HostEntry, HostIdentity, Overview } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { useToast } from "../ui/useToast";
import { identityKey } from "./connectionBrowser";
import { duplicateAliasOf } from "./duplicateAlias";
import { groupAfterRename } from "./connectionMoves";
import { buildGroupTree, decorateHosts, type GroupNode } from "./connectionTreeIndex";
import { ordersAfterMove, type MoveDirection } from "./groupFolders";
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
  // 一括の移動と削除は、保存できたかを返す。保存できたときだけ選択を解く。
  moveConnections: (hosts: readonly HostIdentity[], group: string) => Promise<boolean>;
  deleteConnections: (hosts: readonly HostIdentity[]) => Promise<boolean>;
  // 保存できたかを返す。
  duplicateConnection: (host: HostIdentity) => Promise<boolean>;
  reorderGroups: (orders: ReadonlyMap<string, number>) => Promise<boolean>;
  setGroupHidden: (name: string, hidden: boolean) => Promise<boolean>;
};

// siblingsOf は、グループと同じ親の下に並ぶグループの名前を、表示の順に返す。
function siblingsOf(nodes: readonly GroupNode[], name: string): string[] | null {
  if (nodes.some((node) => node.name === name)) return nodes.map((node) => node.name);
  for (const node of nodes) {
    const found = siblingsOf(node.children, name);
    if (found !== null) return found;
  }
  return null;
}

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
  reorderGroups,
  setGroupHidden,
}: ConnectionListActionsOptions) {
  const t = useTranslate();
  const showToast = useToast();
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
  const groupTree = useMemo(() => (overview === null ? [] : buildGroupTree(overview, decorateHosts(overview))), [overview]);
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

  // 名前を変えたグループ（とその下のグループ）を絞り込んだり編集したりしていたら、新しい名前へ追わせる。
  function followGroupRename(from: string, to: string) {
    if (scope.kind === "group") {
      const renamed = groupAfterRename(scope.name, from, to);
      if (renamed !== null) setScope({ kind: "group", name: renamed });
    }
    setEditingGroup((current) => (current === null ? null : groupAfterRename(current, from, to) ?? current));
  }

  // 消したグループ（とその下のグループ）を絞り込んだり編集したりしていたら、すべての接続先へ戻す。
  function forgetDeletedGroup(name: string) {
    const removed = (group: string) => group === name || group.startsWith(`${name}/`);
    if (scope.kind === "group" && removed(scope.name)) setScope({ kind: "all" });
    setEditingGroup((current) => (current !== null && removed(current) ? null : current));
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
    const siblings = siblingsOf(groupTree, name) ?? [name];
    const move = (direction: MoveDirection) => {
      const orders = ordersAfterMove(siblings, name, direction);
      if (orders !== null) void reorderGroups(orders);
    };
    const hidden = overview?.metadata.groups?.find((group) => group.name === name)?.hidden === true;
    // 直下に接続先があるグループを隠すと、その接続先も一覧から見えなくなるので、隠せるのは入れ物のグループだけ。
    const directMembers = overview?.hosts.filter((host) => host.identity.alias !== "" && (host.group ?? "") === name).length ?? 0;
    return [
      { label: t("tree.newSubgroup"), onSelect: () => setGroupDialog({ kind: "create", parent: name }) },
      { label: t("tree.rename"), onSelect: () => setGroupDialog({ kind: "rename", name }), disabled: movesDisabled },
      ...edit,
      { label: t("tree.moveGroupUp"), onSelect: () => move("up"), disabled: siblings[0] === name },
      { label: t("tree.moveGroupDown"), onSelect: () => move("down"), disabled: siblings.at(-1) === name },
      hidden
        ? { label: t("tree.showGroup"), onSelect: () => void setGroupHidden(name, false) }
        : { label: t("tree.hideGroup"), onSelect: () => void setGroupHidden(name, true), disabled: directMembers > 0, title: directMembers > 0 ? t("tree.hideNeedsEmptyGroup") : undefined },
      { label: t("tree.deleteEllipsis"), tone: "danger", onSelect: () => setGroupDialog({ kind: "delete", name, memberCount }), disabled: movesDisabled },
    ];
  }

  function connectionActionItems(host: HostEntry): ActionMenuItem[] {
    return [
      { label: t("tree.connect"), onSelect: () => void onOpenSSHSession(host.identity.alias) },
      { label: t("tree.rename"), onSelect: () => setRenamingConnection(host), disabled: movesDisabled },
      { label: t("tree.duplicate"), onSelect: () => void duplicateAndTell(host.identity), disabled: movesDisabled },
      { label: t("tree.moveToGroup"), onSelect: () => setMoving([host]), disabled: movesDisabled },
      { label: t("tree.deleteEllipsis"), tone: "danger", onSelect: () => setDeleting([host]), disabled: movesDisabled },
    ];
  }

  // 「…」の［複製］はダイアログを開かず、一覧のどこに増えたかも見えにくいので、できたら知らせる。
  async function duplicateAndTell(host: HostIdentity) {
    if (!(await duplicateConnection(host))) return;
    showToast(t("conn.duplicated", { alias: host.alias, copy: duplicateAliasOf(host.alias) }), { tone: "success" });
  }

  // moveChosenTo と deleteChosen は、ダイアログを開いたまま保存し、保存できたかを返す。閉じるのは呼び手。
  async function moveChosenTo(group: string): Promise<boolean> {
    const saved = await moveConnections((moving ?? []).map((host) => host.identity), group);
    if (saved) clearChecked();
    return saved;
  }

  async function deleteChosen(): Promise<boolean> {
    const saved = await deleteConnections((deleting ?? []).map((host) => host.identity));
    if (saved) clearChecked();
    return saved;
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
    closeMove: () => setMoving(null),
    moveChosenTo,
    deleting,
    confirmDeleting: (hosts: HostEntry[]) => setDeleting(hosts),
    closeDelete: () => setDeleting(null),
    deleteChosen,
    renamingConnection,
    closeConnectionRename: () => setRenamingConnection(null),
  };
}

export type ConnectionListActions = ReturnType<typeof useConnectionListActions>;

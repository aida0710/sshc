import { configApi, type EditRequest, type HostDetail, type Overview } from "../api/config";
import { groupBaseName, isGroupOrDescendant } from "../rules/groupPath";
import { toProblem } from "../api/guards";
import type { NavigateLocationOptions } from "../routing/useSectionRoute";
import type { HostSelection } from "./ConnectionTree";
import { groupAfterRename, movedHostIdentity } from "./connectionMoves";
import type { DraggedConnection, DragPayload } from "./dragdrop";
import type { SaveFeedback } from "./pageState";
import type { SaveAttempt } from "./useConnectionSave";
import { duplicateAliasOf } from "./duplicateAlias";

// A host source for a move: the file it lives in now and that file's
// contents at the time the move was decided.
type MoveSource = { path: string; alias: string; base: string };

// 一覧で選んだ接続先。base は操作のときに読む。
type HostTarget = Pick<DraggedConnection, "path" | "alias">;

type ConnectionManagementOptions = {
  overview: Overview | null;
  selection: HostSelection | null;
  detail: HostDetail | null;
  // 未保存の下書きがあるか、保存済みの接続を読み直しているあいだは、ツリーでの移動を受け付けない。
  movesDisabled: boolean;
  feedback: Pick<SaveFeedback, "setPreview" | "setProblem" | "setLocalError">;
  submit: (request: EditRequest, reselect?: boolean) => Promise<SaveAttempt>;
  reload: () => Promise<Overview | null>;
  followCommittedIdentity: (identity: HostSelection) => void;
  leaveCommittedIdentityUnknown: () => void;
  clearSelection: () => void;
  clearTarget: (options?: NavigateLocationOptions) => boolean;
};

// useConnectionManagement は、管理のパネルとツリーから行う接続の操作（名前の変更、グループや
// ファイルへの移動、コメント、複製、削除）を持つ。移動した接続を選んでいたら、選択も移動先へ追う。
export function useConnectionManagement({
  overview,
  selection,
  detail,
  movesDisabled,
  feedback,
  submit,
  reload,
  followCommittedIdentity,
  leaveCommittedIdentityUnknown,
  clearSelection,
  clearTarget,
}: ConnectionManagementOptions) {
  const { setPreview, setProblem, setLocalError } = feedback;
  const entryPath = overview?.entry.path ?? "config";

  // 開いている接続先への操作（rename から remove まで）は、保存できたかを返す。
  async function rename(newName: string): Promise<boolean> {
    if (detail === null || selection === null) return false;
    setLocalError("");
    return (await submit({
      kind: "rename",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      newAlias: newName,
    })).saved;
  }

  async function comment(text: string): Promise<boolean> {
    if (detail === null || selection === null) return false;
    return (await submit({
      kind: "comment",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      comment: text,
    })).saved;
  }

  // moveHostToGroup moves the host into group, or into the entry file when
  // group is "" (a host without a group lives there). When follow is set,
  // the selection follows the host to wherever the engine put it. Resolves to
  // whether the move was saved.
  async function moveHostToGroup(source: MoveSource, group: string, follow: boolean): Promise<boolean> {
    if (group !== "") {
      const attempt = await submit({ kind: "move", ...source, destinationGroup: group }, false);
      if (!attempt.saved || !follow) return attempt.saved;
      const moved = movedHostIdentity(attempt.overview, source.alias, group);
      if (moved !== undefined) followCommittedIdentity(moved);
      else leaveCommittedIdentityUnknown();
      return true;
    }
    const destination = await configApi.file(entryPath);
    const attempt = await submit({
      kind: "move", ...source, destinationPath: entryPath, destinationBase: destination.contents,
    }, false);
    if (!attempt.saved || !follow) return attempt.saved;
    followCommittedIdentity({ path: entryPath, alias: source.alias });
    return true;
  }

  async function moveToGroup(group: string): Promise<boolean> {
    if (detail === null) return false;
    const source = { path: detail.form.entry.file.path ?? "", alias: detail.form.entry.identity.alias, base: detail.file.contents };
    try {
      return await moveHostToGroup(source, group, true);
    } catch (error) {
      setProblem(toProblem(error));
      return false;
    }
  }

  function selectedHostGroup(): string | undefined {
    return overview?.hosts.find(
      (host) => host.identity.path === selection?.path && host.identity.alias === selection.alias,
    )?.group;
  }

  // followSelectionInto は、開いていた接続先が group へ移ったあと、選択をその新しい識別子へ追わせる。
  function followSelectionInto(nextOverview: Overview | null, group: string) {
    if (selection === null) return;
    const moved = movedHostIdentity(nextOverview, selection.alias, group);
    if (moved !== undefined) followCommittedIdentity(moved);
    else leaveCommittedIdentityUnknown();
  }

  // renameGroupFollowingSelection は、グループ（と、その下のグループ）の名前を変え、開いていた接続先が
  // その中にあれば選択も新しい場所へ追わせる。
  async function renameGroupFollowingSelection(from: string, to: string) {
    const selectedDestinationGroup = groupAfterRename(selectedHostGroup(), from, to);
    const result = await configApi.renameGroup(from, to);
    setPreview(result.preview);
    setProblem(null);
    const nextOverview = await reload();
    if (selectedDestinationGroup !== null) followSelectionInto(nextOverview, selectedDestinationGroup);
  }

  async function renameGroupByDrop(name: string, target: string) {
    const base = groupBaseName(name);
    await renameGroupFollowingSelection(name, target === "" ? base : `${target}/${base}`);
  }

  // renameGroup と deleteGroup は、グループの「…」から呼ぶ。保存できたかを返し、呼び手は
  // 保存できたときだけ絞り込みなどを新しい名前へ追わせる。
  async function renameGroup(from: string, to: string): Promise<boolean> {
    try {
      await renameGroupFollowingSelection(from, to);
      return true;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  async function deleteGroup(name: string, destination: string): Promise<boolean> {
    const selectedGroup = selectedHostGroup();
    const selectionMoves = selectedGroup !== undefined && isGroupOrDescendant(selectedGroup, name);
    try {
      const result = await configApi.deleteGroup(name, destination);
      setPreview(result.preview);
      setProblem(null);
      const nextOverview = await reload();
      if (selectionMoves) followSelectionInto(nextOverview, destination);
      return true;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  // readBases は、選んだ接続先のファイルを1回ずつ読み、保存の前提（base）として返す。
  // 同じファイルにある接続先がいくつあっても中身は1回だけ送り、sshcエンジンはまとめて1回で書き換える。
  async function readBases(items: readonly HostTarget[]): Promise<{ path: string; base: string }[]> {
    const bases: { path: string; base: string }[] = [];
    for (const path of new Set(items.map((item) => item.path))) {
      bases.push({ path, base: (await configApi.file(path)).contents });
    }
    return bases;
  }

  function isSelected(item: HostTarget): boolean {
    return selection?.path === item.path && selection.alias === item.alias;
  }

  // moveConnections は、選んだ接続先を group へまとめて移す。group が "" なら入口のファイルへ戻す。
  // どれか1つのファイルがほかで変わっていれば、sshcエンジンが全体を断り、何も動かない。保存できたかを返す。
  async function moveConnections(items: readonly HostTarget[], group: string): Promise<boolean> {
    if (movesDisabled || items.length === 0) return false;
    try {
      const bases = await readBases(items);
      const entryBase = bases.find((base) => base.path === entryPath)?.base;
      const destination = group === ""
        ? { destinationPath: entryPath, destinationBase: entryBase ?? (await configApi.file(entryPath)).contents }
        : { destinationGroup: group };
      // 開いている接続先の識別子が変わるときだけ選択を追わせ、変わらないときは開いたまま読み直す。
      // 同じファイルのほかの接続先が動くと、開いている接続先の base も変わるため。
      const opened = items.find(isSelected);
      const openedMoves = opened !== undefined && (group === "" ? opened.path !== entryPath : selectedHostGroup() !== group);
      const hosts = items.map((item) => ({ path: item.path, alias: item.alias }));
      const attempt = await submit({ kind: "move_hosts", hosts, bases, ...destination }, !openedMoves);
      if (!attempt.saved) return false;
      if (opened !== undefined && openedMoves) {
        if (group === "") followCommittedIdentity({ path: entryPath, alias: opened.alias });
        else followSelectionInto(attempt.overview, group);
      }
      return true;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  // deleteConnections は、選んだ接続先の Host ブロックをまとめて消す。開いていた接続先が消えたら選択を外し、
  // 残っていれば読み直す。保存できたかを返す。
  async function deleteConnections(items: readonly HostTarget[]): Promise<boolean> {
    if (movesDisabled || items.length === 0) return false;
    try {
      const bases = await readBases(items);
      const deletesOpened = items.some(isSelected);
      const hosts = items.map((item) => ({ path: item.path, alias: item.alias }));
      const attempt = await submit({ kind: "delete_hosts", hosts, bases }, !deletesOpened);
      if (!attempt.saved) return false;
      if (deletesOpened) {
        clearSelection();
        clearTarget({ replace: true });
      }
      return true;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  // renameConnection は、一覧の「…」から接続先の名前を変える。開いている接続先なら選択も新しい名前へ追い、
  // ほかの接続先なら開いている接続先を読み直す（同じファイルなら base が変わるため）。
  // 保存できたかを返す。
  async function renameConnection(item: HostTarget, newAlias: string): Promise<boolean> {
    try {
      const file = await configApi.file(item.path);
      return (await submit({ kind: "rename", path: item.path, alias: item.alias, base: file.contents, newAlias })).saved;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  // 保存できたかを返す。
  async function duplicateConnection(item: HostTarget): Promise<boolean> {
    try {
      const file = await configApi.file(item.path);
      return (await submit({ kind: "duplicate", path: item.path, base: file.contents, alias: item.alias, newAlias: duplicateAliasOf(item.alias) })).saved;
    } catch (error) {
      setProblem(toProblem(error));
      return false;
    }
  }

  // dropOnTree は、一覧からグループへ落とした接続先やグループを動かし、保存できたかを返す。
  async function dropOnTree(payload: DragPayload, target: string): Promise<boolean> {
    if (movesDisabled) return false;
    if (payload.kind === "connections") return moveConnections(payload.items, target);
    try {
      if (payload.kind === "group") {
        await renameGroupByDrop(payload.name, target);
        return true;
      }
      const file = await configApi.file(payload.path);
      const followsSelection = selection?.path === payload.path && selection.alias === payload.alias;
      return await moveHostToGroup({ path: payload.path, alias: payload.alias, base: file.contents }, target, followsSelection);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  async function duplicate(): Promise<boolean> {
    if (detail === null || selection === null) return false;
    setLocalError("");
    return (await submit({
      kind: "duplicate",
      path: selection.path,
      base: detail.file.contents,
      alias: selection.alias,
      newAlias: duplicateAliasOf(selection.alias),
    })).saved;
  }

  async function moveToFile(target: string): Promise<boolean> {
    if (detail === null || selection === null || target === "") return false;
    try {
      const destination = await configApi.file(target);
      const source = selection;
      const attempt = await submit({
        kind: "move",
        path: source.path,
        base: detail.file.contents,
        alias: source.alias,
        destinationPath: target,
        destinationBase: destination.contents,
      }, false);
      if (!attempt.saved) return false;
      followCommittedIdentity({ path: target, alias: source.alias });
      setLocalError("");
      return true;
    } catch (error) {
      setProblem(toProblem(error));
      return false;
    }
  }

  // remove は、開いている接続先を、画面が読み込んだときのファイルを前提に消す。
  async function remove(): Promise<boolean> {
    if (detail === null || selection === null) return false;
    const attempt = await submit({
      kind: "delete_hosts",
      hosts: [{ path: selection.path, alias: selection.alias }],
      bases: [{ path: selection.path, base: detail.file.contents }],
    }, false);
    if (!attempt.saved) return false;
    clearSelection();
    clearTarget({ replace: true });
    return true;
  }

  return {
    rename, comment, moveToGroup, dropOnTree, duplicate, moveToFile, remove,
    moveConnections, deleteConnections, renameConnection, duplicateConnection, renameGroup, deleteGroup,
  };
}

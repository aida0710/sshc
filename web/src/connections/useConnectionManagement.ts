import { configApi, type EditRequest, type HostDetail, type Overview } from "../api/config";
import { toProblem } from "../api/guards";
import type { NavigateLocationOptions } from "../routing/useSectionRoute";
import type { HostSelection } from "./ConnectionTree";
import { groupAfterRename, movedHostIdentity } from "./connectionMoves";
import type { DraggedConnection, DragPayload } from "./dragdrop";
import type { SaveFeedback } from "./pageState";
import type { SaveAttempt } from "./useConnectionSave";

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

  function rename(newName: string) {
    if (detail === null || selection === null) return;
    setLocalError("");
    void submit({
      kind: "rename",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      newAlias: newName,
    });
  }

  function comment(text: string) {
    if (detail === null || selection === null) return;
    void submit({
      kind: "comment",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      comment: text,
    });
  }

  // moveHostToGroup moves the host into group, or into the entry file when
  // group is "" (a host without a group lives there). When follow is set,
  // the selection follows the host to wherever the engine put it.
  async function moveHostToGroup(source: MoveSource, group: string, follow: boolean) {
    if (group !== "") {
      const attempt = await submit({ kind: "move", ...source, destinationGroup: group }, false);
      if (!attempt.saved || !follow) return;
      const moved = movedHostIdentity(attempt.overview, source.alias, group);
      if (moved !== undefined) followCommittedIdentity(moved);
      else leaveCommittedIdentityUnknown();
      return;
    }
    const destination = await configApi.file(entryPath);
    const attempt = await submit({
      kind: "move", ...source, destinationPath: entryPath, destinationBase: destination.contents,
    }, false);
    if (!attempt.saved || !follow) return;
    followCommittedIdentity({ path: entryPath, alias: source.alias });
  }

  async function moveToGroup(group: string) {
    if (detail === null) return;
    const source = { path: detail.form.entry.file.path ?? "", alias: detail.form.entry.identity.alias, base: detail.file.contents };
    try {
      await moveHostToGroup(source, group, true);
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function renameGroupByDrop(name: string, target: string) {
    const base = name.slice(name.lastIndexOf("/") + 1);
    const destinationName = target === "" ? base : `${target}/${base}`;
    const selectedHost = overview?.hosts.find(
      (host) => host.identity.path === selection?.path && host.identity.alias === selection.alias,
    );
    const selectedDestinationGroup = groupAfterRename(selectedHost?.group, name, destinationName);
    const result = await configApi.renameGroup(name, destinationName);
    setPreview(result.preview);
    setProblem(null);
    const nextOverview = await reload();
    if (selection === null || selectedDestinationGroup === null) return;
    const moved = movedHostIdentity(nextOverview, selection.alias, selectedDestinationGroup);
    if (moved !== undefined) followCommittedIdentity(moved);
    else leaveCommittedIdentityUnknown();
  }

  // readBases は、選んだ接続先のファイルを1回ずつ読み、保存の前提（base）として返す。
  // 同じファイルにある接続先は同じ base を持つので、sshcエンジンはまとめて1回で書き換えられる。
  async function readBases(paths: Iterable<string>): Promise<Map<string, string>> {
    const bases = new Map<string, string>();
    for (const path of new Set(paths)) bases.set(path, (await configApi.file(path)).contents);
    return bases;
  }

  function isSelected(item: HostTarget): boolean {
    return selection?.path === item.path && selection.alias === item.alias;
  }

  // moveConnections は、選んだ接続先を group へまとめて移す。group が "" なら入口のファイルへ戻す。
  // どれか1つのファイルがほかで変わっていれば、sshcエンジンが全体を断り、何も動かない。
  async function moveConnections(items: readonly HostTarget[], group: string) {
    if (movesDisabled || items.length === 0) return;
    try {
      const bases = await readBases([...items.map((item) => item.path), ...(group === "" ? [entryPath] : [])]);
      const hosts = items.map((item) => ({ path: item.path, alias: item.alias, base: bases.get(item.path) ?? "" }));
      const destination = group === ""
        ? { destinationPath: entryPath, destinationBase: bases.get(entryPath) ?? "" }
        : { destinationGroup: group };
      const attempt = await submit({ kind: "move_hosts", hosts, ...destination }, false);
      if (!attempt.saved) return;
      const opened = items.find(isSelected);
      if (opened === undefined) return;
      if (group === "") {
        followCommittedIdentity({ path: entryPath, alias: opened.alias });
        return;
      }
      const moved = movedHostIdentity(attempt.overview, opened.alias, group);
      if (moved !== undefined) followCommittedIdentity(moved);
      else leaveCommittedIdentityUnknown();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  // deleteConnections は、選んだ接続先の Host ブロックをまとめて消す。開いていた接続先が消えたら選択を外す。
  async function deleteConnections(items: readonly HostTarget[]) {
    if (movesDisabled || items.length === 0) return;
    try {
      const bases = await readBases(items.map((item) => item.path));
      const hosts = items.map((item) => ({ path: item.path, alias: item.alias, base: bases.get(item.path) ?? "" }));
      const attempt = await submit({ kind: "delete_hosts", hosts }, false);
      if (!attempt.saved || !items.some(isSelected)) return;
      clearSelection();
      clearTarget({ replace: true });
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  // renameConnection は、開いていない接続先の名前を一覧の「…」から変える。
  // 開いている接続先は、選択も新しい名前へ追う rename を使う。
  async function renameConnection(item: HostTarget, newAlias: string) {
    if (selection?.path === item.path && selection.alias === item.alias && detail !== null) {
      rename(newAlias);
      return;
    }
    try {
      const file = await configApi.file(item.path);
      const result = await configApi.save({ kind: "rename", path: item.path, alias: item.alias, base: file.contents, newAlias });
      setPreview(result.preview);
      setProblem(null);
      await reload();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  async function duplicateConnection(item: HostTarget) {
    try {
      const file = await configApi.file(item.path);
      await submit({ kind: "duplicate", path: item.path, base: file.contents, alias: item.alias, newAlias: `${item.alias}-copy` }, false);
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function dropOnTree(payload: DragPayload, target: string) {
    if (movesDisabled) return;
    try {
      if (payload.kind === "group") {
        await renameGroupByDrop(payload.name, target);
        return;
      }
      if (payload.kind === "connections") {
        await moveConnections(payload.items, target);
        return;
      }
      const file = await configApi.file(payload.path);
      const followsSelection = selection?.path === payload.path && selection.alias === payload.alias;
      await moveHostToGroup({ path: payload.path, alias: payload.alias, base: file.contents }, target, followsSelection);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  function duplicate() {
    if (detail === null || selection === null) return;
    setLocalError("");
    void submit({
      kind: "duplicate",
      path: selection.path,
      base: detail.file.contents,
      alias: selection.alias,
      newAlias: `${selection.alias}-copy`,
    });
  }

  async function moveToFile(target: string) {
    if (detail === null || selection === null || target === "") return;
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
      if (!attempt.saved) return;
      followCommittedIdentity({ path: target, alias: source.alias });
      setLocalError("");
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  // remove は、開いている接続先を、画面が読み込んだときのファイルを前提に消す。
  async function remove() {
    if (detail === null || selection === null) return;
    const host = { path: selection.path, alias: selection.alias, base: detail.file.contents };
    const attempt = await submit({ kind: "delete_hosts", hosts: [host] }, false);
    if (!attempt.saved) return;
    clearSelection();
    clearTarget({ replace: true });
  }

  return { rename, comment, moveToGroup, dropOnTree, duplicate, moveToFile, remove, moveConnections, deleteConnections, renameConnection, duplicateConnection };
}

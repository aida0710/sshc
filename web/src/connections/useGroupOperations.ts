import { configApi, type GroupMetadata, type Overview } from "../api/config";
import { toProblem } from "../api/guards";
import type { SaveFeedback } from "./pageState";

type GroupOperationsOptions = {
  overview: Overview | null;
  feedback: Pick<SaveFeedback, "setPreview" | "setProblem">;
  reload: () => Promise<Overview | null>;
};

// useGroupOperations は、Connections 画面から行うグループの作成と、設定・表示順・隠すの保存を持つ。どちらも metadata の
// グループの設定を書き換える操作で、Groups 画面と同じ sshcエンジンの保存を呼ぶ。名前の変更と削除は
// 開いている接続先の場所も変えるので、useConnectionManagement が持つ。
export function useGroupOperations({ overview, feedback, reload }: GroupOperationsOptions) {
  const { setPreview, setProblem } = feedback;

  // 作成と設定の保存は、読み込んだときのグループの設定を base に送る。ほかで変わっていればサーバーが断る。
  async function saveGroups(next: (current: GroupMetadata[]) => GroupMetadata[]): Promise<boolean> {
    if (overview === null) return false;
    const base = overview.metadata.groups ?? [];
    try {
      const result = await configApi.save({ kind: "groups", groupsBase: base, groups: next(base) });
      setPreview(result.preview);
      setProblem(null);
      await reload();
      return true;
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return false;
    }
  }

  // 宣言だけあって metadata に設定が無いグループは、変えるときに足す。
  function changeGroups(changes: ReadonlyMap<string, Partial<GroupMetadata>>): Promise<boolean> {
    return saveGroups((current) => {
      const known = new Set(current.map((group) => group.name));
      const added = [...changes.keys()].filter((name) => !known.has(name)).map((name) => ({ name }));
      return [...current, ...added].map((group) => ({ ...group, ...changes.get(group.name) }));
    });
  }

  return {
    createGroup: (name: string) => saveGroups((current) => [...current, { name }]),
    saveGroup: (updated: GroupMetadata) => changeGroups(new Map([[updated.name, updated]])),
    // reorderGroups は、ordersAfterMove が返した兄弟の表示順を書く。
    reorderGroups: (orders: ReadonlyMap<string, number>) =>
      changeGroups(new Map([...orders].map(([name, order]) => [name, { order }]))),
    setGroupHidden: (name: string, hidden: boolean) => changeGroups(new Map([[name, { hidden }]])),
  };
}

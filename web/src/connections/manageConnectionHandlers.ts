import type { Translate } from "../i18n/context";
import type { ShowToast } from "../ui/toastContext";
import { duplicateAliasOf } from "./duplicateAlias";
import { movedConnectionsMessage } from "./movedMessage";

// 管理パネルの操作が使う、開いている接続先への保存。どれも保存できたかを返す。
type ConnectionManagementSaves = {
  rename: (alias: string) => Promise<boolean>;
  moveToGroup: (group: string) => Promise<boolean>;
  comment: (text: string) => Promise<boolean>;
  duplicate: () => Promise<boolean>;
  moveToFile: (path: string) => Promise<boolean>;
  remove: () => Promise<boolean>;
};

// manageConnectionHandlers は、管理パネルの操作を、保存できたら成功を知らせる形にして返す。
// 管理パネルの下の差分は、開いているタブによっては見えないので、知らせで終わったことを伝える。
// 断られた理由は保存の処理が画面に出すので、ここでは何もしない。
export function manageConnectionHandlers({ saves, alias, t, showToast }: {
  saves: ConnectionManagementSaves;
  alias: string;
  t: Translate;
  showToast: ShowToast;
}) {
  async function tellWhenSaved(saving: Promise<boolean>, message: string) {
    if (await saving) showToast(message, { tone: "success" });
  }

  return {
    onRename: (newAlias: string) => tellWhenSaved(saves.rename(newAlias), t("connectionDialog.renamed", { alias: newAlias })),
    onMoveToGroup: (group: string) => tellWhenSaved(saves.moveToGroup(group), movedConnectionsMessage(t, { aliases: [alias], group })),
    onComment: (text: string) => tellWhenSaved(saves.comment(text), t("host.commentSaved")),
    onDuplicate: () => tellWhenSaved(saves.duplicate(), t("conn.duplicated", { alias, copy: duplicateAliasOf(alias) })),
    onMoveToFile: (path: string) => tellWhenSaved(saves.moveToFile(path), t("tree.moved", { name: alias, destination: path })),
    onDelete: () => tellWhenSaved(saves.remove(), t("bulkDelete.deletedOne", { alias })),
  };
}

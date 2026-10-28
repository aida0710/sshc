import type { Translate } from "../i18n/context";
import type { DragPayload } from "./dragdrop";

// movedConnectionsMessage は、接続先を group へ動かし終えたときの知らせの文を返す。
// group が "" なら、接続先はグループなしへ動いている。
export function movedConnectionsMessage(t: Translate, { aliases, group }: { aliases: readonly string[]; group: string }): string {
  const destination = group === "" ? t("tree.ungrouped") : group;
  if (aliases.length === 1) return t("tree.moved", { name: aliases[0] ?? "", destination });
  return t("tree.movedMany", { count: aliases.length, destination });
}

// droppedMessage は、一覧のグループへ落として動かし終えたときの知らせの文を返す。
// グループを "" へ落とすと、いちばん上の階層へ動く。
export function droppedMessage(t: Translate, { payload, target }: { payload: DragPayload; target: string }): string {
  if (payload.kind === "group") {
    return target === "" ? t("tree.movedGroupToTop", { name: payload.name }) : t("tree.moved", { name: payload.name, destination: target });
  }
  const aliases = payload.kind === "connections" ? payload.items.map((item) => item.alias) : [payload.alias];
  return movedConnectionsMessage(t, { aliases, group: target });
}

import type { Translate } from "../i18n/context";

// resultCountText says how many connections a list shows. Only a filtered list has a second number to compare.
export function resultCountText(t: Translate, visible: number, total: number): string {
  return visible === total ? t("tree.totalCount", { count: total }) : t("tree.resultCount", { visible, total });
}

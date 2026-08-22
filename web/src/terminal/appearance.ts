import type { components } from "../api/schema";

export type TerminalAppearance = components["schemas"]["TerminalAppearance"];

/** Resolved は、決着したあとの見た目である。空は「選ばれていない」。 */
export type Resolved = { readonly palette: string; readonly font: string };

/**
 * resolveAppearance は、接続の選択と全体の選択を重ねる。
 *
 * <p>**接続が勝ち、全体はその下に敷く。** そして**項目ごとに重ねる** ——
 * 接続に配色だけを置いた人の字体が、そこで既定へ戻ってはならない。まとめて
 * 差し替えると、片方を選んだ瞬間にもう片方が消える。
 *
 * <p>空文字は「選ばれていない」である。**空を選択として扱わない** ——
 * 端末の設定画面で「既定へ戻す」を選んだ人は、全体の選択へ戻るのであって、
 * 全体の選択まで消したいわけではない。
 */
export function resolveAppearance(
  forConnection: TerminalAppearance | undefined,
  overall: TerminalAppearance | undefined,
): Resolved {
  const pick = (key: "palette" | "font"): string =>
    forConnection?.[key] !== undefined && forConnection[key] !== ""
      ? forConnection[key]
      : (overall?.[key] ?? "");
  return { palette: pick("palette"), font: pick("font") };
}

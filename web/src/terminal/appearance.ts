import type { components } from "../api/schema";

export type TerminalAppearance = components["schemas"]["TerminalAppearance"];
type HostMetadata = components["schemas"]["HostMetadata"];

/** Resolved は、決着したあとの見た目である。空は「選ばれていない」。 */
export type Resolved = {
  readonly palette: string;
  readonly font: string;
  readonly background: string;
  /**
   * 画像の上にかぶせる濃さ（0〜100）。
   *
   * <p>**undefined は「選んでいない」であって 0 ではない。** 0 は「かぶせない」
   * という選択であり、既定へ落とすべきではない。
   */
  readonly tint: number | undefined;
};

/** defaultTint は、画像を選んだのに濃さを選ばなかった人に効く値である。 */
export const defaultTint = 55;

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
  const pick = (key: "palette" | "font" | "background"): string =>
    forConnection?.[key] !== undefined && forConnection[key] !== ""
      ? forConnection[key]
      : (overall?.[key] ?? "");
  // **濃さは 0 が有効なので、空文字と同じ扱いにできない。** undefined だけが
  // 「選んでいない」である。
  const tint = forConnection?.backgroundTint ?? overall?.backgroundTint;
  return { palette: pick("palette"), font: pick("font"), background: pick("background"), tint };
}

/**
 * chooseAppearance は、接続の見た目を 1 項目だけ書き換えて返す。
 *
 * <p>**他の項目に触らない。** 配色を選んだ操作が字体を消してはならない。
 *
 * <p>**何も選ばれていない節は残さない。** 空を残すと、次に読む者は何か選ばれて
 * いると思う——そして「既定へ戻した」接続の metadata に、何も言っていない節が
 * 積もり続ける。
 */
/**
 * AppearanceChange は、1 項目の書き換えである。
 *
 * <p>**undefined を明示的に渡せる形でなければならない。** exactOptionalPropertyTypes
 * の下では、省略と「undefined と書く」は別のことである——濃さを「選んでいない」へ
 * 戻す操作は、後者でしか表せない。
 */
export type AppearanceChange = { [Key in keyof TerminalAppearance]?: TerminalAppearance[Key] | undefined };

export function chooseAppearance(metadata: HostMetadata, change: AppearanceChange): HostMetadata {
  const merged: AppearanceChange = { ...metadata.appearance, ...change };
  const kept = Object.fromEntries(
    Object.entries(merged).filter(([, value]) => value !== undefined && value !== ""),
  ) as TerminalAppearance;
  const { appearance: _dropped, ...rest } = metadata;
  return Object.keys(kept).length === 0 ? rest : { ...rest, appearance: kept };
}

/**
 * appearanceOf は、画面が持っている 4 つの値を、送れる形へ畳む。
 *
 * <p>**何も選ばれていなければ節ごと送らない。** 空の節を送ると、metadata に
 * 何も言っていない節が残る。
 *
 * <p>濃さは画像を選んでいるときだけ運ぶ。**画像の無い濃さは意味を持たない。**
 */
export function appearanceOf(chosen: {
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
}): { appearance?: TerminalAppearance } {
  const appearance: TerminalAppearance = {
    ...(chosen.palette === "" ? {} : { palette: chosen.palette }),
    ...(chosen.font === "" ? {} : { font: chosen.font }),
    ...(chosen.background === "" ? {} : { background: chosen.background }),
    ...(chosen.background === "" || chosen.tint === undefined ? {} : { backgroundTint: chosen.tint }),
  };
  return Object.keys(appearance).length === 0 ? {} : { appearance };
}

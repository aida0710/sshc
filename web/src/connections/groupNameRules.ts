import type { Translate } from "../i18n/context";
import { maxGroupSegments } from "../rules/generated";
import { isValidGroupName } from "../rules/rules";

export function joinGroup(parent: string, segment: string): string {
  return parent === "" ? segment : `${parent}/${segment}`;
}

// groupSegmentProblem は、グループの名前の欄に入れた値の問題を、画面に出す文で返す（無ければ ""）。
// 欄には親を除いた最後の1段だけを入れてもらう。階層は「中にグループを作成」とドラッグで決める。
// current は名前を変えるグループで、今の名前のままなら重複に数えない。taken は小文字にした既存の名前。
export function groupSegmentProblem(
  t: Translate,
  { parent, value, current, taken }: { parent: string; value: string; current: string | null; taken: ReadonlySet<string> },
): string {
  if (value.includes("/")) return t("groupDialog.noSlash");
  if (!isValidGroupName(value)) return t("groupDialog.invalidCharacters");
  const full = joinGroup(parent, value);
  if (!isValidGroupName(full)) return t("groupDialog.tooDeep", { count: maxGroupSegments });
  if (full !== current && taken.has(full.toLowerCase())) return t("groupDialog.nameTaken");
  return "";
}

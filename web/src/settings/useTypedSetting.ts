import { useEffect, useState } from "react";
import { useDraftProblem, useSettingsDraft } from "./settingsDraftContext";

// ParsedSetting は、打った文字を読んだ結果。使える値か、使えない理由。
export type ParsedSetting<Value> = { value: Value } | { problem: string };

// useTypedSetting は、打って入れる設定の文字を持つ。使える値になるたびに下書きへ渡し、使えないあいだは理由を返して
// ［保存］を止める。途中の文字（空、範囲外の数）も消さずに持つ。text は下書きの値を文字にしたもの。
export function useTypedSetting<Value>({ text, parse, onChange }: {
  text: string;
  parse: (typed: string) => ParsedSetting<Value>;
  onChange: (value: Value) => void;
}) {
  const { discarded } = useSettingsDraft();
  // typed は打った文字。打っていないあいだは null で、下書きの値に従う。
  const [typed, setTyped] = useState<string | null>(null);
  const shown = typed ?? text;
  const parsed = typed === null ? null : parse(typed);
  const problem = parsed !== null && "problem" in parsed ? parsed.problem : "";
  useDraftProblem(problem);

  useEffect(() => setTyped(null), [discarded]);

  return {
    shown,
    problem,
    type: (next: string) => {
      setTyped(next);
      const read = parse(next);
      if ("value" in read) onChange(read.value);
    },
    // 使える値なら、欄を離れたときに書き方を整える（"007" を "7" にするなど）。
    leave: () => {
      if (problem === "") setTyped(null);
    },
  };
}

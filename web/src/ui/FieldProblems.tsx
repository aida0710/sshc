import { useTranslate } from "../i18n/context";

// FieldProblem は、欄の名前と、その欄の値の問題（どう直すか）。tone "notice" は誤りではなく、欄を書けない理由などの注意。
export type FieldProblem = { id: string; field: string; text: string; tone?: "danger" | "notice" };

// FieldProblems は、欄を並べたまとまりの下に、問題のある欄の名前と直し方を幅いっぱいで並べる。欄のすぐ下に出すと、
// ポートのような狭い欄では文が何行にも折り返す（利用者が選んだ形、2026-11-24）。どの欄かは、その欄の赤い枠
// （index.css の aria-invalid の決まり）と、文の頭の欄の名前で示す。入力欄は aria-describedby で id を指す。
// 欄から離れたときに出る文なので、読み上げで割り込まない（移った先の欄の読み上げを遮らないため）。
export function FieldProblems({ problems, className = "" }: { problems: readonly FieldProblem[]; className?: string }) {
  const t = useTranslate();
  const shown = problems.filter((problem) => problem.text !== "");
  if (shown.length === 0) return null;
  return (
    <div className={`flex flex-col gap-0.5 ${className}`}>
      {shown.map((problem) => (
        <p key={problem.id} id={problem.id} className={`text-xs leading-[18px] ${problem.tone === "notice" ? "text-notice-ink" : "text-danger"}`}>
          {t("form.fieldProblem", { field: problem.field, problem: problem.text })}
        </p>
      ))}
    </div>
  );
}

import { createContext, useCallback, useContext, useEffect, useId, useState } from "react";
import type { DescribableControlProps } from "./describeControl";

// FieldErrorContext は、行（FieldRow）の中の部品が、自分の値の誤りを行に伝える口。1つの行に欄が並ぶとき（分割転送の
// 3つの数など）、どの欄の誤りも行の下に出す。
export const FieldErrorContext = createContext<((id: string, error: string) => void) | null>(null);

const noErrors: ReadonlyMap<string, string> = new Map();

// useReportedFieldErrors は、FieldErrorContext で中の部品から伝えられた誤りを、文の id ごとに持つ。誤りの文を出すのは
// 呼び手（FieldRow、マシンの一覧の行）で、文には reported の id を付ける（入力欄の aria-describedby がそれを指す）。
export function useReportedFieldErrors(): { reported: ReadonlyMap<string, string>; reportError: (id: string, error: string) => void } {
  const [reported, setReported] = useState<ReadonlyMap<string, string>>(noErrors);
  const reportError = useCallback((id: string, error: string) => setReported((current) => {
    if ((current.get(id) ?? "") === error) return current;
    const next = new Map(current);
    if (error === "") next.delete(id);
    else next.set(id, error);
    return next;
  }), []);
  return { reported, reportError };
}

// useFieldError は、値の誤りを行の下に出し、入力欄に付ける属性を返す。description は行から渡された説明で、誤りの文を
// 足して返す。誤りが無いときは error を "" にする。
export function useFieldError(error: string, description: DescribableControlProps): DescribableControlProps {
  const id = useId();
  const report = useContext(FieldErrorContext);
  useEffect(() => {
    if (report === null) return;
    report(id, error);
    return () => report(id, "");
  }, [error, id, report]);
  if (error === "") return description;
  return {
    "aria-invalid": true,
    "aria-describedby": [description["aria-describedby"], report === null ? undefined : id].filter(Boolean).join(" ") || undefined,
  };
}

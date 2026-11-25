import { createContext, useContext, useEffect, useId } from "react";
import type { DescribableControlProps } from "./describeControl";

// FieldErrorContext は、行（FieldRow）の中の部品が、自分の値の誤りを行に伝える口。1つの行に欄が並ぶとき（分割転送の
// 3つの数など）、どの欄の誤りも行の下に出す。
export const FieldErrorContext = createContext<((id: string, error: string) => void) | null>(null);

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

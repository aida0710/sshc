import { cloneElement, Fragment, isValidElement, type ReactElement, type ReactNode } from "react";

export type DescribableControlProps = {
  "aria-describedby"?: string | undefined;
  "aria-invalid"?: boolean | undefined;
};

// describeControl は、入力欄に hint と誤りの文を aria-describedby で結び付け、誤りが
// あれば aria-invalid を付ける。入力欄の横に出す文を、フォーカスが入ったときにも
// 読ませるためである。
//
// 子の props に差し込むので、子が素の入力要素でない部品なら、その部品が
// DescribableControlProps を受け取って中の入力欄へ渡す必要がある（PasswordInput が
// この形）。受け取らない部品に渡した値は黙って捨てられる。
export function describeControl(
  control: ReactNode,
  { descriptionIds, invalid = false }: { descriptionIds: (string | undefined)[]; invalid?: boolean },
): ReactNode {
  if (!isValidElement(control) || control.type === Fragment) return control;
  const element = control as ReactElement<DescribableControlProps>;
  const describedBy = [element.props["aria-describedby"], ...descriptionIds].filter(Boolean).join(" ");
  if (describedBy === "" && !invalid) return control;
  return cloneElement(element, {
    ...(describedBy === "" ? {} : { "aria-describedby": describedBy }),
    ...(invalid ? { "aria-invalid": true } : {}),
  });
}

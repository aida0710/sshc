import type { ReactNode } from "react";
import { Icon, type IconName } from "./icons";

type EmptyStateProps = {
  icon: IconName;
  title: string;
  detail?: string;
  action?: ReactNode;
};

// EmptyState は、画面の主な領域にまだ何も無いとき（接続先を選んでいない、ホストを選んでいない）に出す形である。
// 印・見出し・説明・操作の順に、枠を付けずに真ん中へ並べる（見つからないページと同じ形）。見出しは、
// 画面の見出し（h2）の下に置くので h3 にする。
export function EmptyState({ icon, title, detail, action }: EmptyStateProps) {
  return (
    <section className="m-auto flex max-w-[420px] flex-col items-center px-6 py-10 text-center" role="status">
      <span aria-hidden="true" className="mb-3.5 flex size-11 items-center justify-center rounded-xl bg-well text-ink-muted">
        <Icon name={icon} className="size-[22px]" />
      </span>
      <h3 className="text-lg font-semibold text-ink">{title}</h3>
      {detail === undefined ? null : <p className="mt-1.5 text-[13px] leading-5 text-ink-muted">{detail}</p>}
      {action === undefined ? null : <div className="mt-3.5">{action}</div>}
    </section>
  );
}

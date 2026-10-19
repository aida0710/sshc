import type { ReactNode } from "react";
import { Icon, type IconName } from "./icons";

type EmptyStateProps = {
  icon: IconName;
  title: string;
  detail?: string;
  action?: ReactNode;
};

// EmptyState は、画面の主な領域にまだ何も無いとき（接続先を選んでいない、ホストを選んでいない）に出す形である。
// 印・見出し・説明・操作の順に並べ、どの画面でも同じ見た目にする。
export function EmptyState({ icon, title, detail, action }: EmptyStateProps) {
  return (
    <section className="m-auto flex max-w-sm flex-col items-center rounded-lg bg-tree px-8 py-10 text-center" role="status">
      <span aria-hidden="true" className="mb-4 flex size-14 items-center justify-center rounded-lg bg-card text-accent shadow-sm">
        <Icon name={icon} className="size-7" />
      </span>
      <h2 className="text-lg font-semibold text-ink">{title}</h2>
      {detail === undefined ? null : <p className="mt-1 text-sm leading-6 text-ink-muted">{detail}</p>}
      {action === undefined ? null : <div className="mt-4">{action}</div>}
    </section>
  );
}

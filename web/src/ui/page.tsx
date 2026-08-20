import type { ReactNode } from "react";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div className="min-w-0">
        <h2 className="text-xl font-semibold text-ink">{title}</h2>
        <p className="mt-1 max-w-3xl text-sm leading-6 text-ink-muted">{description}</p>
      </div>
      {/* **縮めない箱に、縮まないものを入れない。** shrink-0 の中では子の
          max-w-full が効かない——親が max-content 幅を取るからである。狭い画面
          では箱ごと畳めるようにする。 */}
      {/* **縮めない箱に、縮まないものを入れない。** shrink-0 の中では子の
          max-w-full が効かない——親が max-content 幅を取るからである。狭い面では
          箱ごと畳んで、広い面では今までどおり右端に寄せる。 */}
      {actions === undefined ? null : (
        <div className="flex w-full min-w-0 flex-wrap gap-2 sm:w-auto sm:shrink-0">{actions}</div>
      )}
    </header>
  );
}

export function MetricCard({
  label,
  value,
  detail,
  attention = false,
}: {
  label: string;
  value: string | number;
  detail?: string;
  attention?: boolean;
}) {
  return (
    <div className={`rounded-xl border bg-card p-4 ${attention ? "border-notice-line" : "border-line"}`}>
      <p className={`text-xs font-medium uppercase tracking-wide ${attention ? "text-notice-ink" : "text-ink-muted"}`}>
        {label}
      </p>
      <p className="mt-1 text-2xl font-semibold text-ink">{value}</p>
      {detail === undefined ? null : <p className="mt-1 text-xs text-ink-muted">{detail}</p>}
    </div>
  );
}

export function MetricGrid({ children }: { children: ReactNode }) {
  return <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</div>;
}

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
    <header className="flex flex-wrap items-end justify-between gap-3 border-b border-line pb-3">
      <div className="min-w-0">
        <h2 className="text-xl font-semibold tracking-tight text-ink">{title}</h2>
        <p className="mt-1 max-w-3xl text-sm leading-5 text-ink-muted">{description}</p>
      </div>


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
    <div className={`px-4 py-3.5 ${attention ? "bg-notice" : "bg-card"}`}>
      <p className={`text-xs font-medium tracking-wide ${attention ? "text-notice-ink" : "text-ink-muted"}`}>
        {label}
      </p>
      <p className="mt-1 text-xl font-semibold tracking-tight text-ink">{value}</p>
      {detail === undefined ? null : <p className="mt-1 text-xs text-ink-muted">{detail}</p>}
    </div>
  );
}

export function MetricGrid({ children }: { children: ReactNode }) {
  return (
    <div className="sshc-card grid gap-px overflow-hidden rounded bg-line sm:grid-cols-2 lg:grid-cols-3">
      {children}
    </div>
  );
}

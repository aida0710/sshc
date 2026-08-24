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
      <div className="relative min-w-0 pl-4 before:absolute before:inset-y-1 before:left-0 before:w-0.5 before:rounded-full before:bg-accent">
        <h2 className="text-2xl font-semibold tracking-tight text-ink">{title}</h2>
        <p className="mt-1 max-w-3xl text-sm leading-6 text-ink-muted">{description}</p>
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
    <div className="sshc-card grid gap-px overflow-hidden rounded-xl bg-line sm:grid-cols-2 lg:grid-cols-3">
      {children}
    </div>
  );
}

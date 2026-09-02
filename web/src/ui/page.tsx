import type { ComponentPropsWithoutRef, ReactNode } from "react";

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
  compact = false,
  icon,
  className = "",
}: {
  label: string;
  value: string | number;
  detail?: string;
  attention?: boolean;
  compact?: boolean;
  icon?: ReactNode;
  className?: string;
}) {
  return (
    <div className={`${compact ? "flex items-center justify-between gap-4 px-4 py-2.5" : icon === undefined ? "px-4 py-3.5" : "flex items-center gap-3 px-4 py-3.5"} ${attention ? "bg-notice" : "bg-card"} ${className}`}>
      {icon === undefined ? null : <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-surface text-accent">{icon}</span>}
      <div className={icon === undefined ? "contents" : "min-w-0 grow"}>
        <dt className={`text-xs font-medium tracking-wide ${attention ? "text-notice-ink" : "text-ink-muted"}`}>
          {label}
        </dt>
        <dd className={`${compact ? "font-mono text-sm" : "mt-1 text-xl tracking-tight"} font-semibold ${attention ? "text-notice-ink" : "text-ink"}`}>{value}</dd>
        {detail === undefined ? null : <p className="mt-1 text-xs text-ink-muted">{detail}</p>}
      </div>
    </div>
  );
}

export function MetricGrid({
  children,
  className = "",
  ...rest
}: ComponentPropsWithoutRef<"dl">) {
  return (
    <dl className={`sshc-card grid gap-px overflow-hidden rounded-lg bg-line sm:grid-cols-2 lg:grid-cols-3 ${className}`} {...rest}>
      {children}
    </dl>
  );
}

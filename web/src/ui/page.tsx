import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// PageHeader is the top of a page: its name, a line on what it is for, and its main actions on the right.
// On a phone the app's top bar already shows the section's name, so the heading is only for screen
// readers there, unless the page is named apart from its section (titleOnPhone, as in Settings).
export function PageHeader({
  title,
  headingId,
  description,
  actions,
  titleOnPhone = false,
  className = "",
}: {
  title: string;
  headingId?: string;
  description?: string | undefined;
  actions?: ReactNode;
  titleOnPhone?: boolean;
  className?: string;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  return (
    <header className={`flex flex-wrap items-start justify-between gap-4 ${className}`}>
      <div className="min-w-0 flex-1">
        <h2 id={headingId} className={compact && !titleOnPhone ? "sr-only" : "text-xl font-semibold leading-[29px] text-ink"}>{title}</h2>
        {description === undefined ? null : <p className="mt-0.5 max-w-3xl text-xs leading-[18px] text-ink-faint">{description}</p>}
      </div>
      {actions === undefined ? null : (
        <div className="flex w-full min-w-0 flex-wrap gap-2 sm:w-auto sm:shrink-0">{actions}</div>
      )}
    </header>
  );
}

// SummaryStats is the row of small counts under a page's heading.
export function SummaryStats({ children, className = "", ...rest }: ComponentPropsWithoutRef<"dl">) {
  return (
    <dl className={`flex flex-wrap gap-2 ${className}`} {...rest}>
      {children}
    </dl>
  );
}

// SummaryStat is one of those counts. attention paints it amber, for a count that asks for a look.
export function SummaryStat({ label, value, attention = false }: { label: string; value: string | number; attention?: boolean }) {
  return (
    <div className={`inline-flex h-[30px] items-center gap-2 rounded-md px-3 text-[12.5px] ${attention ? "bg-notice text-notice-ink" : "bg-well text-ink-muted"}`}>
      <dt>{label}</dt>
      <dd className={`font-mono text-[13px] font-semibold ${attention ? "text-notice-ink" : "text-ink"}`}>{value}</dd>
    </div>
  );
}

import { useId, type ComponentPropsWithoutRef, type ReactNode } from "react";
import { mobileViewportQuery, useMediaQuery } from "./useMediaQuery";

// PageHeader is the top of a page: its name, a line on what it is for, and its main actions on the right.
// On a phone the app's top bar already shows the section's name, so the heading is only for screen
// readers there, unless the page is named apart from its section (titleOnPhone, as in Settings).
export function PageHeader({
  title,
  headingId,
  meta,
  description,
  actions,
  titleOnPhone = false,
  className = "",
}: {
  title: string;
  headingId?: string;
  // meta goes beside the name in small faint type, such as how many connections there are.
  meta?: string | undefined;
  description?: string | undefined;
  actions?: ReactNode;
  titleOnPhone?: boolean;
  className?: string;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  return (
    <header className={`flex flex-wrap items-start justify-between gap-4 ${className}`}>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-baseline gap-2.5">
          <h2 id={headingId} className={compact && !titleOnPhone ? "sr-only" : "text-xl font-semibold leading-[29px] text-ink"}>{title}</h2>
          {meta === undefined ? null : <span className={compact && !titleOnPhone ? "sr-only" : "shrink-0 text-[12.5px] text-ink-faint"}>{meta}</span>}
        </div>
        {description === undefined ? null : <p className="mt-0.5 max-w-3xl text-xs leading-[18px] text-ink-faint">{description}</p>}
      </div>
      {actions === undefined ? null : (
        <div className="flex w-full min-w-0 flex-wrap justify-end gap-2 sm:w-auto sm:shrink-0">{actions}</div>
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

// PageSection is one part of a page, set off from the part above by a line, as in the design: a heading
// with details beside it (meta, such as a file path) and controls on its right (actions), a hint under
// it, then the contents. On a phone each part is a card of its own instead.
export function PageSection({
  heading,
  headingId,
  meta,
  actions,
  hint,
  children,
}: {
  heading: string;
  headingId?: string;
  meta?: ReactNode;
  actions?: ReactNode;
  hint?: string;
  children: ReactNode;
}) {
  const compact = useMediaQuery(mobileViewportQuery);
  const fallbackId = useId();
  const id = headingId ?? fallbackId;
  return (
    <section aria-labelledby={id} className={compact ? "rounded-[14px] border border-line bg-sidebar px-3.5 py-3" : "border-t border-line pt-5"}>
      <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-2">
        <h3 id={id} className="text-sm font-semibold text-ink">{heading}</h3>
        {meta === undefined ? null : <span className="min-w-0 truncate font-mono text-[11.5px] text-ink-faint">{meta}</span>}
        {actions === undefined ? null : <div className="ml-auto flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
      {hint === undefined ? null : <p className="-mt-1.5 mb-3 text-xs leading-[18px] text-ink-faint">{hint}</p>}
      {children}
    </section>
  );
}

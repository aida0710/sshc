import type { SelectHTMLAttributes } from "react";
import { Icon, type IconName } from "./icons";

// ChevronSelect is a native select drawn with the design's chevron instead of the browser's arrow, which
// differs by platform. leadingIcon goes before the chosen value, as in the corner of the vault screens.
// Where the browser can, the select is as wide as the chosen value rather than its longest option.
export function ChevronSelect({
  leadingIcon,
  className = "",
  children,
  ...select
}: SelectHTMLAttributes<HTMLSelectElement> & { leadingIcon?: IconName }) {
  return (
    <span className="relative inline-flex items-center">
      {leadingIcon === undefined ? null : (
        <Icon name={leadingIcon} className="pointer-events-none absolute left-2.5 size-3.5 text-ink-muted" />
      )}
      <select
        {...select}
        className={`h-[30px] appearance-none rounded-md border border-line pr-[31px] text-[13px] text-ink [field-sizing:content] ${leadingIcon === undefined ? "pl-2.5" : "pl-8"} ${className}`}
      >
        {children}
      </select>
      <Icon name="chevronDown" className="pointer-events-none absolute right-2.5 size-[13px] text-ink-muted" />
    </span>
  );
}

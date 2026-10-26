import type { SelectHTMLAttributes } from "react";
import { Icon, type IconName } from "./icons";

// ChevronSelect is a compact native select (index.css gives every select the design's chevron).
// leadingIcon goes before the chosen value, as in the corner of the vault screens. Where the browser
// can, the select is as wide as the chosen value rather than its longest option.
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
        className={`h-[30px] rounded-md border border-line text-[13px] text-ink [field-sizing:content] ${leadingIcon === undefined ? "pl-2.5" : "pl-8"} ${className}`}
      >
        {children}
      </select>
    </span>
  );
}

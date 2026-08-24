import type { ButtonHTMLAttributes, ReactNode } from "react";
import { dangerAction, hintText, primaryAction, secondaryAction } from "./form";

export function Card({ children, padded = false }: { children: ReactNode; padded?: boolean }) {
  return (
    <div
      className={`sshc-card overflow-hidden rounded-xl bg-card ${
        padded ? "flex flex-col gap-3 p-3" : ""
      }`}
    >
      {children}
    </div>
  );
}

export function Row({
  label,
  children,
  hint,
  warning,
  action,
}: {
  label: string;
  children: ReactNode;
  hint?: string | undefined;
  warning?: string | undefined;
  action?: ReactNode;
}) {
  return (
    <div className="border-t border-hairline first:border-t-0">
      <div className="flex items-center gap-3 px-3 py-2">
        <label className="flex min-w-0 flex-1 items-center gap-3">
          <span className="w-32 shrink-0 text-sm text-ink-muted">{label}</span>
          <span className="ml-auto flex min-w-0 flex-1 justify-end">{children}</span>
        </label>
        {action === undefined ? null : <span className="shrink-0">{action}</span>}
      </div>
      {hint === undefined ? null : <p className={`px-3 pb-2 ${hintText}`}>{hint}</p>}
      {warning === undefined ? null : (
        <p role="status" className="px-3 pb-2 text-xs text-notice-ink">
          {warning}
        </p>
      )}
    </div>
  );
}

export function Notice({ children, tone = "notice" }: { children: ReactNode; tone?: "notice" | "danger" }) {
  const danger = tone === "danger";
  return (
    <p
      role={danger ? "alert" : "status"}
      className={
        danger
          ? "flex items-center gap-2 rounded-lg border border-control-line px-3 py-2 text-sm text-danger"
          : "flex items-center gap-2 rounded-lg border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink"
      }
    >
      {children}
    </p>
  );
}

export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <div role="group" aria-label={label} className="flex gap-0.5 rounded-lg bg-select-fill p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className={`rounded-md px-2.5 py-1 text-xs transition-colors ${
            value === option.value ? "bg-card text-ink shadow-sm" : "text-ink-muted hover:text-ink"
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

type ButtonProps = { kind?: "primary" | "secondary" | "danger" } & ButtonHTMLAttributes<HTMLButtonElement>;

export function Button({ kind = "secondary", className = "", type = "button", ...rest }: ButtonProps) {
  const base = kind === "primary" ? primaryAction : kind === "danger" ? dangerAction : secondaryAction;
  return <button type={type} className={`${base} ${className}`} {...rest} />;
}

import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ElementType,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { dangerAction, hintText, primaryAction, secondaryAction } from "./form";

type CardElement = "div" | "section" | "article" | "dl" | "ul";

type CardProps = {
  as?: CardElement;
  padded?: boolean;
  radius?: "sm" | "md" | "lg";
  tone?: "card" | "notice" | "subtle";
  overflow?: "hidden" | "visible" | "x-auto";
} & HTMLAttributes<HTMLElement>;

export function Card({
  as,
  children,
  padded = false,
  radius = "lg",
  tone = "card",
  overflow = "hidden",
  className = "",
  ...rest
}: CardProps) {
  const Component = (as ?? "div") as ElementType;
  const radiusClass = radius === "sm" ? "rounded" : radius === "md" ? "rounded-md" : "rounded-lg";
  const toneClass = tone === "notice" ? "bg-notice" : tone === "subtle" ? "bg-surface-subtle" : "bg-card";
  const overflowClass = overflow === "visible" ? "overflow-visible" : overflow === "x-auto" ? "overflow-x-auto" : "overflow-hidden";
  return (
    <Component
      className={`sshc-card ${overflowClass} ${radiusClass} ${toneClass} ${
        padded ? "flex flex-col gap-3 p-3" : ""
      } ${className}`}
      {...rest}
    >
      {children}
    </Component>
  );
}

export function Row({
  label,
  children,
  hint,
  warning,
  action,
  stackOnNarrow = false,
  interactiveChildren = false,
}: {
  label: string;
  children: ReactNode;
  hint?: string | undefined;
  warning?: string | undefined;
  action?: ReactNode;
  stackOnNarrow?: boolean;
  interactiveChildren?: boolean;
}) {
  const rowLayout = stackOnNarrow
    ? "flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2"
    : "flex items-center gap-3 px-3 py-2";
  const labelLayout = stackOnNarrow
    ? "flex min-w-0 flex-1 flex-col items-stretch gap-1.5 sm:flex-row sm:items-center sm:gap-3"
    : "flex min-w-0 flex-1 items-center gap-3";
  const contents = (
    <>
      <span
        className={`${stackOnNarrow ? "w-full sm:w-32" : "w-32"} shrink-0 text-sm text-ink-muted`}
      >
        {label}
      </span>
      <span
        className={`${stackOnNarrow ? "w-full sm:w-auto" : "ml-auto"} flex min-w-0 flex-1 justify-end`}
      >
        {children}
      </span>
    </>
  );
  return (
    <div className="border-t border-hairline first:border-t-0">
      <div className={rowLayout}>
        {interactiveChildren ? (
          <div className={labelLayout}>{contents}</div>
        ) : (
          <label className={labelLayout}>{contents}</label>
        )}
        {action === undefined ? null : (
          <span className="shrink-0">{action}</span>
        )}
      </div>
      {hint === undefined ? null : (
        <p className={`px-3 pb-2 ${hintText}`}>{hint}</p>
      )}
      {warning === undefined ? null : (
        <p role="status" className="px-3 pb-2 text-xs text-notice-ink">
          {warning}
        </p>
      )}
    </div>
  );
}

export function Notice({
  children,
  tone = "notice",
}: {
  children: ReactNode;
  tone?: "notice" | "danger";
}) {
  const danger = tone === "danger";
  return (
    <p
      role={danger ? "alert" : "status"}
      className={
        danger
          ? "flex items-center gap-2 rounded-md border border-control-line px-3 py-2 text-sm text-danger"
          : "flex items-center gap-2 rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink"
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
    <div
      role="group"
      aria-label={label}
      className="flex overflow-hidden rounded-md border border-control-line bg-control"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className={`border-r border-control-line px-2.5 py-1 text-xs transition-colors last:border-r-0 ${
            value === option.value
              ? "bg-select-fill text-ink"
              : "text-ink-muted hover:text-ink"
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

type ButtonProps = {
  kind?: "primary" | "secondary" | "danger";
} & ButtonHTMLAttributes<HTMLButtonElement>;

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    kind = "secondary",
    className = "",
    type = "button",
    ...rest
  },
  ref,
) {
  const base =
    kind === "primary"
      ? primaryAction
      : kind === "danger"
        ? dangerAction
        : secondaryAction;
  return <button ref={ref} type={type} className={`${base} ${className}`} {...rest} />;
});

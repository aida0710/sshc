import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ElementType,
  type HTMLAttributes,
  type ReactNode,
  useId,
} from "react";
import { describeControl } from "./describeControl";
import { dangerAction, dangerTextAction, hintText, primaryAction, secondaryAction, smallSecondaryAction } from "./form";
import { Spinner } from "./Spinner";

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
  const hintId = useId();
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
        {describeControl(children, { descriptionIds: [hint === undefined ? undefined : hintId] })}
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
        <p id={hintId} className={`px-3 pb-2 ${hintText}`}>{hint}</p>
      )}
      {warning === undefined ? null : (
        <p role="status" className="px-3 pb-2 text-xs text-notice-ink">
          {warning}
        </p>
      )}
    </div>
  );
}

const noticeToneClasses = {
  notice: "border-notice-line bg-notice text-notice-ink",
  danger: "border-control-line text-danger",
  success: "border-live/40 bg-live/10 text-live",
} as const;

// Notice is the one way a panel tells the user something went wrong, needs
// attention, or has just been done ("success": saved, started, restored).
// "compact" fits it into dense lists and side panes.
export function Notice({
  children,
  tone = "notice",
  compact = false,
}: {
  children: ReactNode;
  tone?: keyof typeof noticeToneClasses;
  compact?: boolean;
}) {
  const spacing = compact ? "px-2 py-1.5 text-xs" : "px-3 py-2 text-sm";
  return (
    <p
      role={tone === "danger" ? "alert" : "status"}
      className={`flex items-center gap-2 rounded-md border ${spacing} ${noticeToneClasses[tone]}`}
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
      className="inline-flex rounded-md border border-line bg-well p-0.5"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className={`h-[26px] rounded-[6px] px-2.5 text-[12.5px] transition-colors ${
            value === option.value
              ? "bg-select-fill text-ink"
              : "text-ink-muted hover:bg-hover hover:text-ink"
          }`}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

type ButtonProps = (
  // small は知らせの中や状態の行の端に置く低いボタン。今は secondary だけにある。
  | { kind?: "secondary"; small?: boolean }
  | { kind: "primary" | "danger" | "dangerText"; small?: never }
) & {
  // busy は、このボタンで始めた処理を待っている間。文言は変えずに回転の印を添え、
  // aria-busy を付けて押せなくする（薄くはしない。index.css の決まり）。
  busy?: boolean;
} & ButtonHTMLAttributes<HTMLButtonElement>;

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    kind = "secondary",
    small = false,
    busy = false,
    disabled,
    className = "",
    type = "button",
    children,
    ...rest
  },
  ref,
) {
  const base =
    kind === "primary"
      ? primaryAction
      : kind === "danger"
        ? dangerAction
        : kind === "dangerText"
          ? dangerTextAction
          : small
            ? smallSecondaryAction
            : secondaryAction;
  // 回転の印と文言を横に並べるため、待っている間だけ flex にする。いつも flex にすると、
  // 幅いっぱいに広げたボタンの文言が左に寄る。
  const busyLayout = busy ? "inline-flex items-center justify-center gap-2" : "";
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled === true || busy}
      aria-busy={busy || undefined}
      className={`${base} ${busyLayout} ${className}`}
      {...rest}
    >
      {busy ? <Spinner /> : null}
      {children}
    </button>
  );
});

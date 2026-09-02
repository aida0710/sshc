import type { ReactNode } from "react";

export const control =
  "w-full rounded-md border border-control-line bg-control px-2 py-1.5 text-sm text-ink " +
  "placeholder:text-ink-faint focus:border-accent focus:outline-none " +
  "disabled:border-line disabled:text-ink-faint";

export const narrowControl = control.replace("w-full", "w-40");

export const autoControl = control.replace("w-full", "w-auto");

export const primaryAction =
  "whitespace-nowrap rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink " +
  "hover:brightness-110 disabled:bg-line disabled:text-ink-faint";

export const secondaryAction =
  "whitespace-nowrap rounded-md border border-control-line bg-card px-3 py-1.5 text-sm text-ink " +
  "hover:bg-select-fill disabled:text-ink-faint";

export const dangerAction =
  "whitespace-nowrap rounded-md border border-control-line px-3 py-1.5 text-sm text-danger " +
  "hover:bg-select-fill";

export const fieldLabel = "text-xs font-medium tracking-wide text-ink-muted";
export const hintText = "text-xs text-ink-muted";
export const sectionCard =
  "flex flex-col gap-4 rounded-lg border border-line bg-card p-4";
export const sectionHeading = "text-sm font-medium text-ink";

export const tableHeadRow =
  "border-b border-line text-xs uppercase tracking-wide text-ink-muted";
export const tableHeadCell = "py-2 pr-3 text-left font-medium";

type FieldProps = {
  label: string;
  hint?: string;
  children: ReactNode;
  interactiveChildren?: boolean;
};

export function Field({
  label,
  hint,
  children,
  interactiveChildren = false,
}: FieldProps) {
  const contents = (
    <>
      <span className={fieldLabel}>{label}</span>
      {children}
    </>
  );
  return (
    <div className="flex flex-col gap-1">
      {interactiveChildren ? (
        <div className="flex flex-col gap-1">{contents}</div>
      ) : (
        <label className="flex flex-col gap-1">{contents}</label>
      )}
      {hint === undefined ? null : <span className={hintText}>{hint}</span>}
    </div>
  );
}

type CheckboxFieldProps = {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
};

export function CheckboxField({
  label,
  checked,
  onChange,
  disabled = false,
}: CheckboxFieldProps) {
  return (
    <label
      className={`flex items-start gap-2 text-sm ${disabled ? "text-ink-faint" : "text-ink"}`}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        className="mt-0.5 h-4 w-4 shrink-0 accent-accent"
      />
      <span>{label}</span>
    </label>
  );
}

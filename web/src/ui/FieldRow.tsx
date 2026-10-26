import { useId, type ReactNode } from "react";
import { describeControl } from "./describeControl";

// FieldRow is one input of a form, as in the design: its name on the left, the control and a hint on the
// right, and a line above it unless it is the first row. Narrow screens put the name above the control.
// label is a ReactNode so a row can add a quiet note under its name, such as "optional".
export function FieldRow({
  label,
  hint,
  interactiveChildren = false,
  children,
}: {
  label: ReactNode;
  hint?: string | undefined;
  // interactiveChildren is for a control made of several parts (a field with a show button); the row then
  // does not wrap them in one <label>, which would send every click to the first part.
  interactiveChildren?: boolean;
  children: ReactNode;
}) {
  const hintId = useId();
  const name = <span className="pt-1.5 text-[13.5px] font-medium text-ink">{label}</span>;
  const control = <span className="flex min-w-0 flex-col">{describeControl(children, { descriptionIds: [hint === undefined ? undefined : hintId] })}</span>;
  // The name and the control share a subgrid, so the <label> holds just them and the hint below stays out
  // of the control's accessible name (it is its description instead).
  const pair = "grid gap-2 lg:col-span-2 lg:grid-cols-subgrid lg:gap-x-6";
  return (
    <div className="grid gap-y-1.5 border-t border-line py-3.5 first:border-t-0 first:pt-0.5 lg:grid-cols-[160px_minmax(0,1fr)] lg:gap-x-6">
      {interactiveChildren ? <div className={pair}>{name}{control}</div> : <label className={pair}>{name}{control}</label>}
      {hint === undefined ? null : <span id={hintId} className="text-xs leading-[18px] text-ink-faint lg:col-start-2">{hint}</span>}
    </div>
  );
}

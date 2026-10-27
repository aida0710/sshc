import { useId, type ReactNode } from "react";
import { describeControl } from "./describeControl";

// The two columns of a form: the names, then the controls. FieldRowNote lines up with the second. The names'
// column fits twelve characters, so names such as 現在のマスターパスワード do not break mid-word. Other
// name-and-value rows (FactRows, Appearance) use the same columns so the pages line up.
export const formColumns = "lg:grid-cols-[184px_minmax(0,1fr)] lg:gap-x-6";

// FieldRow is one input of a form, as in the design: its name on the left, the control and a hint on the
// right, and a line above it unless it is the first row. Narrow screens put the name above the control.
// label is a ReactNode so a row can add a quiet note under its name, such as "optional".
export function FieldRow({
  label,
  hint,
  warning,
  actions,
  interactiveChildren = false,
  children,
}: {
  label: ReactNode;
  hint?: string | undefined;
  // warning is what is wrong with the value, in amber under the hint. The control reads it as a description
  // and is marked invalid. It is announced politely when it changes: some warnings are there from the start
  // (a value written in a form the editor cannot change), and an alert would interrupt as the form opens.
  warning?: string | undefined;
  // actions are buttons about the field (such as going back to the inherited value), under the hint and
  // outside the <label>, so a click on them does not land on the control.
  actions?: ReactNode;
  // interactiveChildren is for a control made of several parts (a field with a show button); the row then
  // does not wrap them in one <label>, which would send every click to the first part.
  interactiveChildren?: boolean;
  children: ReactNode;
}) {
  const hintId = useId();
  const warningId = useId();
  const name = <span className="pt-1.5 text-[13.5px] font-medium text-ink">{label}</span>;
  const control = (
    <span className="flex min-w-0 flex-col">
      {describeControl(children, {
        descriptionIds: [hint === undefined ? undefined : hintId, warning === undefined ? undefined : warningId],
        invalid: warning !== undefined,
      })}
    </span>
  );
  // The name and the control share a subgrid, so the <label> holds just them and the hint below stays out
  // of the control's accessible name (it is its description instead).
  const pair = "grid gap-2 lg:col-span-2 lg:grid-cols-subgrid lg:gap-x-6";
  return (
    <div className={`grid gap-y-1.5 border-t border-line py-3.5 first:border-t-0 first:pt-0.5 ${formColumns}`}>
      {interactiveChildren ? <div className={pair}>{name}{control}</div> : <label className={pair}>{name}{control}</label>}
      {hint === undefined ? null : <span id={hintId} className="text-xs leading-[18px] text-ink-faint lg:col-start-2">{hint}</span>}
      {warning === undefined ? null : <span id={warningId} aria-live="polite" className="text-xs leading-[18px] text-notice-ink lg:col-start-2">{warning}</span>}
      {actions === undefined ? null : <div className="flex flex-wrap gap-2 lg:col-start-2">{actions}</div>}
    </div>
  );
}

// FieldRowNote is something under a row that belongs with the controls, such as a notice about the value
// or a box that asks for the vault's password; it lines up with the controls' column.
export function FieldRowNote({ children }: { children: ReactNode }) {
  return (
    <div className={`grid pb-3.5 ${formColumns}`}>
      <div className="flex min-w-0 flex-col gap-2 lg:col-start-2">{children}</div>
    </div>
  );
}

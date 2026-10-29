import { useId, type ReactNode } from "react";
import { describeControl } from "./describeControl";

// The two columns of a form: the names, then the controls. FieldRowNote lines up with the second. The names'
// column fits twelve characters, so names such as 現在のマスターパスワード do not break mid-word. Other
// name-and-value rows (FactRows) use the same columns so the pages line up.
export const formColumns = "lg:grid-cols-[184px_minmax(0,1fr)] lg:gap-x-6";

// The connection editor sits beside the list, so it is narrower than a page and its width does not follow the
// window. Its rows take a 132px names' column once the editor itself is 24rem wide (editorContainer marks the
// element they measure), and are not ruled, so a tab of settings fits on one screen.
export const editorContainer = "@container/editor";

type Density = "page" | "editor";

// A page row's one column below lg is minmax(0, 1fr), not the implicit auto: a control that is itself an
// auto-fill grid (Appearance's themes) needs a definite width to count its columns, or it lays all of them in
// one row past the edge.
const looks: Record<Density, { row: string; name: string; pair: string; second: string }> = {
  page: {
    row: `grid grid-cols-[minmax(0,1fr)] gap-y-1.5 border-t border-line py-3.5 first:border-t-0 first:pt-0.5 ${formColumns}`,
    name: "pt-1.5 text-[13.5px] font-medium text-ink",
    pair: "grid gap-2 lg:col-span-2 lg:grid-cols-subgrid lg:gap-x-6",
    second: "lg:col-start-2",
  },
  editor: {
    row: "grid gap-y-1 py-1.5 @sm/editor:grid-cols-[132px_minmax(0,1fr)] @sm/editor:gap-x-3.5",
    name: "text-[13px] text-ink-muted @sm/editor:pt-1.5",
    pair: "grid gap-1.5 @sm/editor:col-span-2 @sm/editor:grid-cols-subgrid @sm/editor:gap-x-3.5",
    second: "@sm/editor:col-start-2",
  },
};

// FieldRow is one input of a form, as in the design: its name on the left, the control and a hint on the
// right, and a line above it unless it is the first row. Narrow screens put the name above the control.
// label is a ReactNode so a row can add a quiet note under its name, such as "optional".
export function FieldRow({
  label,
  hint,
  warning,
  actions,
  interactiveChildren = false,
  density = "page",
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
  // density "editor" is for the connection editor's rows (see editorContainer).
  density?: Density;
  children: ReactNode;
}) {
  const look = looks[density];
  const hintId = useId();
  const warningId = useId();
  const name = <span className={look.name}>{label}</span>;
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
  return (
    <div className={look.row}>
      {interactiveChildren ? <div className={look.pair}>{name}{control}</div> : <label className={look.pair}>{name}{control}</label>}
      {hint === undefined ? null : <span id={hintId} className={`text-xs leading-[18px] text-ink-faint ${look.second}`}>{hint}</span>}
      {warning === undefined ? null : <span id={warningId} aria-live="polite" className={`text-xs leading-[18px] text-notice-ink ${look.second}`}>{warning}</span>}
      {actions === undefined ? null : <div className={`flex flex-wrap gap-2 ${look.second}`}>{actions}</div>}
    </div>
  );
}

// FieldRowNote is something under a row that belongs with the controls, such as a notice about the value
// or a box that asks for the vault's password; it lines up with the controls' column.
export function FieldRowNote({ density = "page", children }: { density?: Density; children: ReactNode }) {
  const look = looks[density];
  const columns = density === "page" ? `pb-3.5 ${formColumns}` : "pb-1.5 @sm/editor:grid-cols-[132px_minmax(0,1fr)] @sm/editor:gap-x-3.5";
  return (
    <div className={`grid ${columns}`}>
      <div className={`flex min-w-0 flex-col gap-2 ${look.second}`}>{children}</div>
    </div>
  );
}

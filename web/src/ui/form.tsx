import { useId, type ReactNode } from "react";
import { describeControl } from "./describeControl";

// The border keeps the stronger control line: appearance.spec holds it to a 2.5:1 contrast, so a field
// is found without its label.
const controlLook =
  "rounded-md border border-control-line bg-control px-2.5 py-[5px] text-ui leading-5 text-ink " +
  "placeholder:text-ink-faint " +
  "disabled:border-line disabled:text-ink-faint";

// sizedControl は、共通の入力欄の見た目を、幅のクラス（"w-[110px]" など）とともに返す。control に
// 幅のクラスを足すと w-full と競り合うので、幅を変える入力欄はこれを使う。Tailwind はクラス名を字面で
// 拾うので、幅のクラスは呼ぶ側のファイルに書き切る。
// 文字の大きさは変えられない。text-ui は index.css の定義で Tailwind の text-xs や text-sm より後ろに並ぶので、
// 後ろに足した大きさは効かない。
export function sizedControl(widthClass: string): string {
  return `${widthClass} ${controlLook}`;
}

export const control = sizedControl("w-full");

export const narrowControl = sizedControl("w-40");

// 押せないときの見た目（薄くする、載せても光らない）は index.css の :disabled の決まりが
// どのボタンにも同じく付けるので、ここでは書かない。
// touchLinkTarget makes a link on a line of its own 44px tall on a phone, its text in the middle of the height.
export const touchLinkTarget = "mobile-layout:inline-flex mobile-layout:min-h-11 mobile-layout:items-center";

export const primaryAction =
  "whitespace-nowrap rounded-md bg-accent px-3.5 py-1.5 text-ui font-medium leading-5 text-accent-ink " +
  "transition-[background-color,box-shadow,filter] duration-150 hover:brightness-110 active:brightness-95";

const secondaryLook =
  "whitespace-nowrap rounded-md border border-line bg-well leading-5 text-ink " +
  "transition-colors duration-150 hover:border-accent/50 hover:bg-hover active:bg-select-fill";

export const secondaryAction = `${secondaryLook} px-3.5 py-[5px] text-ui`;

// smallSecondaryAction is 26px tall, for a button inside a notice or at the end of a status line.
export const smallSecondaryAction = `${secondaryLook} px-2.5 py-[2px] text-xs`;

export const dangerAction =
  "whitespace-nowrap rounded-md border border-line px-3.5 py-[5px] text-ui leading-5 text-danger " +
  "transition-colors duration-150 hover:border-danger/50 hover:bg-danger/10";

// dangerTextAction is a removal offered in a list row: red words only, so a list of rows is not a column
// of red boxes. A dialog that confirms the removal uses dangerAction.
export const dangerTextAction =
  "whitespace-nowrap rounded-md px-2 py-1 text-xs font-medium text-danger transition-colors duration-150 hover:bg-danger/10";

// iconAction is a button that shows only an icon, such as closing a panel. The caller adds its size.
export const iconAction =
  "inline-flex shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-hover hover:text-ink";

// codeBlock is a command or its output shown as code. The caller adds how tall it may grow and whether long
// lines wrap.
export const codeBlock = "overflow-auto rounded-md bg-code-bg p-2 text-xs text-code-fg";

// dialogHeading is the heading of every dialog, so a confirmation and a form read as the same kind of window.
export const dialogHeading = "text-base font-semibold text-ink";

export const fieldLabel = "text-xs text-ink-muted";
export const hintText = "text-xs text-ink-muted";
// sectionHeading is the heading of a section inside a page, a pane or a dialog: as heavy as the headings of
// PageSection and the settings, so that every section of the application is headed alike.
export const sectionHeading = "text-sm font-semibold text-ink";

export const tableHeadRow =
  "border-b border-line text-xs text-ink-faint";
export const tableHeadCell = "whitespace-nowrap px-2.5 pb-2 text-left font-normal";
// tableCell is a cell of a list row, spaced as in the design.
export const tableCell = "px-2.5 py-[9px] align-middle";

type FieldProps = {
  label: string;
  hint?: string;
  error?: string | undefined;
  children: ReactNode;
  interactiveChildren?: boolean;
};

export function Field({
  label,
  hint,
  error,
  children,
  interactiveChildren = false,
}: FieldProps) {
  const hintId = useId();
  const errorId = useId();
  const hasError = error !== undefined && error !== "";
  const describedChild = describeControl(children, {
    descriptionIds: [hint === undefined ? undefined : hintId, hasError ? errorId : undefined],
    invalid: hasError,
  });
  const contents = (
    <>
      <span className={fieldLabel}>{label}</span>
      {describedChild}
    </>
  );
  return (
    <div className="flex flex-col gap-1">
      {interactiveChildren ? (
        <div className="flex flex-col gap-1">{contents}</div>
      ) : (
        <label className="flex flex-col gap-1">{contents}</label>
      )}
      {hint === undefined ? null : <span id={hintId} className={hintText}>{hint}</span>}
      {!hasError ? null : (
        <span id={errorId} role="alert" className="text-xs text-danger">{error}</span>
      )}
    </div>
  );
}

type CheckboxFieldProps = {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  tone?: "default" | "notice" | "danger";
  className?: string;
};

// CheckboxField is a check box with its label, spaced as the design's check rows. A hint goes under the
// label inside the same <label>, so the whole block is one touch target, but the box is named by the label
// alone and the hint is its description.
// A phone makes the label 44px tall. A label with no hint is usually one line, which the padding puts in the middle of
// that height: at the top, it would leave a gap under the line that looks like a stray margin.
export function CheckboxField({
  label,
  hint,
  checked,
  onChange,
  disabled = false,
  tone = "default",
  className = "",
}: CheckboxFieldProps) {
  const labelId = useId();
  const hintId = useId();
  const toneClass = tone === "notice" ? "text-notice-ink" : tone === "danger" ? "text-danger" : disabled ? "text-ink-faint" : "text-ink";
  return (
    <label
      className={`flex items-start gap-2.5 text-sm ${hint === undefined ? "mobile-layout:py-3" : ""} ${toneClass} ${className}`}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
        {...(hint === undefined ? {} : { "aria-labelledby": labelId, "aria-describedby": hintId })}
        className="mt-0.5 h-4 w-4 shrink-0 accent-accent"
      />
      <span>
        <span id={labelId}>{label}</span>
        {hint === undefined ? null : <span id={hintId} className="mt-[3px] block text-xs leading-[18px] text-ink-faint">{hint}</span>}
      </span>
    </label>
  );
}

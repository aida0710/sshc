import { useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { control, hintText } from "./form";
import { Button } from "./surface";
import { ModalShell } from "./ModalShell";

export function InputDialog({
  id,
  heading,
  description,
  label,
  initialValue = "",
  submitLabel,
  cancelLabel,
  validate,
  onSubmit,
  onCancel,
  inputMode,
  returnFocusRef,
}: {
  id: string;
  heading: string;
  description?: ReactNode;
  label: string;
  initialValue?: string;
  submitLabel: string;
  cancelLabel: string;
  validate?: (value: string) => string;
  onSubmit: (value: string) => void;
  onCancel: () => void;
  inputMode?: "text" | "numeric";
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const [value, setValue] = useState(initialValue);
  const [touched, setTouched] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const normalized = value.trim();
  const validation = useMemo(() => validate?.(normalized) ?? "", [normalized, validate]);
  const errorID = `${id}-error`;
  const inputID = `${id}-input`;
  const descriptionID = description === undefined ? undefined : `${id}-description`;

  function submit() {
    setTouched(true);
    if (validation !== "") return;
    onSubmit(normalized);
  }

  return (
    <ModalShell
      labelledBy={id}
      {...(descriptionID === undefined ? {} : { describedBy: descriptionID })}
      onDismiss={onCancel}
      initialFocusRef={inputRef}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex w-full max-w-md flex-col gap-4 rounded-lg p-4 sm:p-5"
    >
      <div>
        <h2 id={id} className="text-base font-semibold text-ink">{heading}</h2>
        {description === undefined ? null : <div id={descriptionID} className={`mt-1 ${hintText}`}>{description}</div>}
      </div>
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <div className="flex flex-col gap-1">
          <label htmlFor={inputID} className="text-xs font-medium tracking-wide text-ink-muted">{label}</label>
          <input
            id={inputID}
            ref={inputRef}
            value={value}
            inputMode={inputMode}
            aria-invalid={touched && validation !== ""}
            aria-describedby={touched && validation !== "" ? errorID : undefined}
            onChange={(event) => setValue(event.target.value)}
            className={control}
          />
          {touched && validation !== "" ? <span id={errorID} role="alert" className="text-xs text-danger">{validation}</span> : null}
        </div>
        <div className="flex justify-end gap-2">
          <Button onClick={onCancel}>{cancelLabel}</Button>
          <Button kind="primary" type="submit">{submitLabel}</Button>
        </div>
      </form>
    </ModalShell>
  );
}

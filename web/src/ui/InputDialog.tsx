import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { control, hintText } from "./form";
import { Button } from "./surface";
import { ModalShell } from "./ModalShell";

// InputDialog は、名前など1つの値を尋ねる。
//
// 入れた値で始めた処理の結果をどこに出すかは、ConfirmDialog と同じく呼び手が決める。
// - 閉じずに待つ: onSubmit から Promise を返し、失敗の文を error で渡す。待つ間は確定の
//   ボタンに回転の印を出し、両方のボタンとダイアログを閉じる操作を止める。失敗したら
//   入力欄へフォーカスを戻し、直してそのままやり直せるようにする。成功したら呼び手が閉じる。
// - 先に閉じる: onSubmit の中でダイアログを閉じてから処理を始め、結果は背面の画面に出す。
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
  error = "",
}: {
  id: string;
  heading: string;
  description?: ReactNode;
  label: string;
  initialValue?: string;
  submitLabel: string;
  cancelLabel: string;
  validate?: (value: string) => string;
  onSubmit: (value: string) => void | Promise<unknown>;
  onCancel: () => void;
  inputMode?: "text" | "numeric";
  returnFocusRef?: RefObject<HTMLElement | null>;
  error?: string;
}) {
  const [value, setValue] = useState(initialValue);
  const [touched, setTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const normalized = value.trim();
  const validation = useMemo(() => validate?.(normalized) ?? "", [normalized, validate]);
  const errorId = `${id}-error`;
  const inputId = `${id}-input`;
  const descriptionId = description === undefined ? undefined : `${id}-description`;
  const failureId = `${id}-failure`;
  const describedBy = [descriptionId, error === "" ? undefined : failureId].filter((part) => part !== undefined).join(" ");
  useFocusInputAfterFailure(submitting, error, inputRef);

  function submit() {
    if (submitting) return;
    setTouched(true);
    if (validation !== "") return;
    const pending = onSubmit(normalized);
    if (pending === undefined) return;
    setSubmitting(true);
    // 失敗の文は呼び手が error で渡す。ここでは終わるのを待つだけにする。
    void pending.catch(() => undefined).finally(() => setSubmitting(false));
  }

  return (
    <ModalShell
      labelledBy={id}
      {...(describedBy === "" ? {} : { describedBy })}
      onDismiss={onCancel}
      dismissible={!submitting}
      initialFocusRef={inputRef}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex w-full max-w-md flex-col gap-4 rounded-lg p-4 sm:p-5"
    >
      <div>
        <h2 id={id} className="text-base font-semibold text-ink">{heading}</h2>
        {description === undefined ? null : <div id={descriptionId} className={`mt-1 ${hintText}`}>{description}</div>}
      </div>
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <div className="flex flex-col gap-1">
          <label htmlFor={inputId} className="text-xs font-medium tracking-wide text-ink-muted">{label}</label>
          <input
            id={inputId}
            ref={inputRef}
            value={value}
            readOnly={submitting}
            inputMode={inputMode}
            aria-invalid={touched && validation !== ""}
            aria-describedby={touched && validation !== "" ? errorId : undefined}
            onChange={(event) => setValue(event.target.value)}
            className={control}
          />
          {touched && validation !== "" ? <span id={errorId} role="alert" className="text-xs text-danger">{validation}</span> : null}
        </div>
        {error === "" ? null : <p id={failureId} role="alert" className="text-sm text-danger">{error}</p>}
        <div className="flex justify-end gap-2">
          <Button disabled={submitting} onClick={onCancel}>{cancelLabel}</Button>
          <Button kind="primary" type="submit" busy={submitting}>{submitLabel}</Button>
        </div>
      </form>
    </ModalShell>
  );
}

// 待っている間は確定のボタンが押せなくなり、フォーカスが外れることがある。失敗して
// ダイアログが開いたままなら、入力欄へ戻して直せるようにする。
function useFocusInputAfterFailure(submitting: boolean, error: string, inputRef: RefObject<HTMLInputElement | null>) {
  const wasSubmitting = useRef(false);
  useEffect(() => {
    if (submitting) {
      wasSubmitting.current = true;
      return;
    }
    if (!wasSubmitting.current) return;
    wasSubmitting.current = false;
    if (error !== "") inputRef.current?.focus();
  }, [submitting, error, inputRef]);
}

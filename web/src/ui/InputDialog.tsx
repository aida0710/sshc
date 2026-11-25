import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { usePendingResult } from "./usePendingResult";
import { control, dialogHeading, Field, hintText } from "./form";
import { Button, Notice } from "./surface";
import { ModalShell } from "./ModalShell";
import { hasOnScreenKeyboard, type asciiTextEntry } from "./softKeyboard";

// InputDialog は、名前など1つの値を尋ねる。
//
// 入れた値で始めた処理の結果をどこに出すかは、ConfirmDialog と同じく呼び手が決める。
// - 閉じずに待つ: onSubmit から Promise を返し、失敗の文を error で渡す。待つ間は確定の
//   ボタンが回転の印と busyLabel（「変更中…」）に替わり、両方のボタンとダイアログを閉じる操作を
//   止める。失敗したら入力欄へフォーカスを戻し、直してそのままやり直せるようにする。成功したら
//   呼び手が閉じる。
// - 先に閉じる: onSubmit の中でダイアログを閉じてから処理を始め、結果は背面の画面に出す。
//   待つ間が無いので、busyLabel に null を渡す。
export function InputDialog({
  id,
  heading,
  description,
  label,
  initialValue = "",
  submitLabel,
  busyLabel,
  cancelLabel,
  validate,
  onSubmit,
  onCancel,
  textEntry,
  returnFocusRef,
  error = "",
}: {
  id: string;
  heading: string;
  description?: ReactNode;
  label: string;
  initialValue?: string;
  submitLabel: string;
  busyLabel: string | null;
  cancelLabel: string;
  validate?: (value: string) => string;
  onSubmit: (value: string) => void | Promise<unknown>;
  onCancel: () => void;
  // textEntry says how a phone's keyboard opens for the value (softKeyboard.ts).
  textEntry?: typeof asciiTextEntry;
  returnFocusRef?: RefObject<HTMLElement | null>;
  error?: string;
}) {
  const [value, setValue] = useState(initialValue);
  const [touched, setTouched] = useState(false);
  const submission = usePendingResult();
  const submitting = submission.pending;
  const inputRef = useRef<HTMLInputElement>(null);
  const normalized = value.trim();
  const validation = useMemo(() => validate?.(normalized) ?? "", [normalized, validate]);
  const descriptionId = description === undefined ? undefined : `${id}-description`;
  const failureId = `${id}-failure`;
  const describedBy = [descriptionId, error === "" ? undefined : failureId].filter((part) => part !== undefined).join(" ");
  useFocusInputAfterFailure(submitting, error, inputRef);

  function submit() {
    if (submitting) return;
    setTouched(true);
    if (validation !== "") return;
    submission.watch(onSubmit(normalized));
  }

  return (
    <ModalShell
      labelledBy={id}
      {...(describedBy === "" ? {} : { describedBy })}
      onDismiss={onCancel}
      dismissible={!submitting}
      initialFocusRef={inputRef}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex w-full max-w-md flex-col gap-4 p-4 sm:p-5"
    >
      <div>
        <h2 id={id} className={dialogHeading}>{heading}</h2>
        {description === undefined ? null : <div id={descriptionId} className={`mt-1 ${hintText}`}>{description}</div>}
      </div>
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        {/* 誤りは確定を押したあとだけ出す。打っている途中から赤くしない。 */}
        <Field label={label} error={touched ? validation : ""}>
          <input
            {...textEntry}
            ref={inputRef}
            value={value}
            readOnly={submitting}
            onChange={(event) => setValue(event.target.value)}
            className={control}
          />
        </Field>
        {error === "" ? null : <Notice tone="danger" id={failureId}>{error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button disabled={submitting} onClick={onCancel}>{cancelLabel}</Button>
          <Button kind="primary" type="submit" busy={submitting} busyLabel={busyLabel}>{submitLabel}</Button>
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
    if (error !== "" && !hasOnScreenKeyboard()) inputRef.current?.focus();
  }, [submitting, error, inputRef]);
}

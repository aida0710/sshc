import { useRef, type ReactNode, type RefObject } from "react";
import { dangerAction, secondaryAction } from "./form";
import { ModalShell } from "./ModalShell";
export function ConfirmDialog({
  id,
  heading,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  returnFocusRef,
}: {
  id: string;
  heading: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  return (
    <ModalShell
      labelledBy={id}
      onDismiss={onCancel}
      initialFocusRef={cancelRef}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="flex w-full max-w-sm flex-col gap-3 rounded-lg p-4"
    >
        <h2 id={id} className="text-sm font-medium text-ink">
          {heading}
        </h2>
        {body}
        <div className="flex justify-end gap-2">
          <button ref={cancelRef} type="button" onClick={onCancel} className={secondaryAction}>
            {cancelLabel}
          </button>
          <button type="button" onClick={onConfirm} className={dangerAction}>
            {confirmLabel}
          </button>
        </div>
    </ModalShell>
  );
}

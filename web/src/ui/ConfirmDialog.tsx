import { useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { dangerAction, secondaryAction } from "./form";
import { useDismissibleLayer } from "./useDismissibleLayer";
export function ConfirmDialog({
  id,
  heading,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
}: {
  id: string;
  heading: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  useDismissibleLayer({
    open: true,
    containerRefs: [dialogRef],
    onDismiss: onCancel,
    closeOnOutside: false,
    initialFocusRef: cancelRef,
    trapFocus: true,
  });

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/75 p-4">
      <div
        ref={dialogRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        className="flex w-full max-w-sm flex-col gap-3 rounded-lg border border-control-line bg-card p-4"
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
      </div>
    </div>,
    document.body,
  );
}

import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { dangerAction, secondaryAction } from "./form";
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

  useEffect(() => {
    cancelRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onCancel]);

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/75 p-4">
      <div
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

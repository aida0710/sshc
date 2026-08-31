import { useRef, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useDismissibleLayer, type DismissReason } from "./useDismissibleLayer";

type ModalPlacement = "center" | "sheet" | "palette";

const placementClasses: Record<ModalPlacement, string> = {
  center: "items-center justify-center p-4",
  sheet: "items-end justify-center p-3 sm:items-center",
  palette: "items-start justify-center px-3 pt-[10vh] md:pt-[14vh]",
};

export function ModalShell({
  open = true,
  labelledBy,
  describedBy,
  children,
  onDismiss,
  closeOnOutside = false,
  initialFocusRef,
  returnFocusRef,
  placement = "center",
  panelClassName = "w-full max-w-lg",
  backdropClassName = "",
  zIndexClassName = "z-50",
}: {
  open?: boolean;
  labelledBy: string;
  describedBy?: string;
  children: ReactNode;
  onDismiss: (reason: DismissReason) => void;
  closeOnOutside?: boolean;
  initialFocusRef?: RefObject<HTMLElement | null>;
  returnFocusRef?: RefObject<HTMLElement | null>;
  placement?: ModalPlacement;
  panelClassName?: string;
  backdropClassName?: string;
  zIndexClassName?: string;
}) {
  const panelRef = useRef<HTMLElement>(null);

  useDismissibleLayer({
    open,
    containerRefs: [panelRef],
    onDismiss,
    closeOnOutside,
    ...(initialFocusRef === undefined ? {} : { initialFocusRef }),
    ...(returnFocusRef === undefined ? {} : { returnFocusRef }),
    trapFocus: true,
  });

  if (!open) return null;

  return createPortal(
    <div
      className={`fixed inset-0 ${zIndexClassName} flex bg-canvas/80 backdrop-blur-[2px] ${placementClasses[placement]} ${backdropClassName}`}
    >
      <section
        ref={panelRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        className={`sshc-card border border-control-line bg-card shadow-2xl ${panelClassName}`}
      >
        {children}
      </section>
    </div>,
    document.body,
  );
}

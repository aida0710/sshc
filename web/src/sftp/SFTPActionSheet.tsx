import { useId, type ReactNode, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { iconAction } from "../ui/form";
import { Icon } from "../ui/icons";
import { ModalShell } from "../ui/ModalShell";

// SFTPActionSheet is a menu on a phone: a sheet from the bottom of the screen, with its name and a way to close it.
// footer is what the person may want to know while choosing, under the choices.
export function SFTPActionSheet({ heading, onDismiss, returnFocusRef, footer, children }: {
  heading: string;
  onDismiss: () => void;
  returnFocusRef: RefObject<HTMLElement | null>;
  footer?: ReactNode;
  children: ReactNode;
}) {
  const t = useTranslate();
  const headingId = useId();
  return (
    <ModalShell labelledBy={headingId} onDismiss={onDismiss} closeOnOutside returnFocusRef={returnFocusRef} placement="sheet" panelClassName="flex max-h-[80dvh] w-full max-w-lg flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-line px-4 py-1">
        <h3 id={headingId} title={heading} className="min-w-0 truncate font-medium">{heading}</h3>
        <button type="button" aria-label={t("sftp.close")} onClick={onDismiss} className={`${iconAction} size-12`}><Icon name="close" className="size-5" /></button>
      </div>
      <div role="menu" className="min-h-0 space-y-1 overflow-y-auto overscroll-contain p-2">{children}</div>
      {footer === undefined ? null : <div className="shrink-0 border-t border-line px-2 py-1.5">{footer}</div>}
    </ModalShell>
  );
}

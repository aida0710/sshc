import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslate } from "../i18n/context";
import { connectionLocation } from "../routing/connectionRoute";
import { Icon } from "../ui/icons";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";
import { useAnchoredMenu } from "../ui/useAnchoredMenu";

type ConnectionActionsProps = {
  alias: string;
  path: string;
  busy: boolean;
  opening?: boolean;
  onOpenSettings: (location: string) => void;
  onConnect: () => void;
};

export function ConnectionActions({
  alias,
  path,
  busy,
  opening = false,
  onOpenSettings,
  onConnect,
}: ConnectionActionsProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useDismissibleLayer({
    open,
    containerRefs: [rootRef, menuRef],
    onDismiss: () => setOpen(false),
    returnFocusRef: triggerRef,
  });
  useMenuKeyboard({ open, menuRef, onClose: () => setOpen(false) });

  useAnchoredMenu({ open, anchorRef: triggerRef, menuRef });

  const settingsLocation = connectionLocation({ path, alias, panel: "Basic", advanced: "Jump" });

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        ref={triggerRef}
        type="button"
        aria-label={t("home.connectionActions", { alias })}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex size-10 items-center justify-center rounded-md border border-control-line bg-card text-ink hover:bg-select-fill md:size-9"
      >
        <Icon name="moreHorizontal" className="size-5" />
      </button>
      {open ? createPortal(
        <div
          ref={menuRef}
          role="menu"
          className="fixed z-50 w-56 overflow-y-auto rounded-lg border border-line bg-card p-1 shadow-lg"
        >
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onOpenSettings(settingsLocation);
            }}
            className="block min-h-10 w-full rounded-md px-3 py-2 text-left text-sm text-ink hover:bg-select-fill focus:bg-select-fill focus:outline-none md:min-h-0"
          >
            {t("home.openConnectionSettings")}
          </button>
          <button
            type="button"
            role="menuitem"
            disabled={busy}
            onClick={() => {
              setOpen(false);
              onConnect();
            }}
            className="block min-h-10 w-full rounded-md px-3 py-2 text-left text-sm text-ink hover:bg-select-fill focus:bg-select-fill focus:outline-none disabled:text-ink-faint md:min-h-0"
          >
            {opening ? t("home.opening") : t("home.connect")}
          </button>
        </div>,
        document.body,
      ) : null}
    </div>
  );
}

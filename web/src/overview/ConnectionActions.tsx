import { useEffect, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { connectionLocation } from "../routing/connectionRoute";

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

  useEffect(() => {
    if (!open) return;

    const closeOutside = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setOpen(false);
      triggerRef.current?.focus();
    };
    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

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
        className="flex size-9 items-center justify-center rounded-md border border-control-line bg-card text-lg leading-none text-ink hover:bg-select-fill"
      >
        <span aria-hidden="true">…</span>
      </button>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 z-20 mt-1 min-w-48 rounded-lg border border-line bg-card p-1 shadow-lg"
        >
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onOpenSettings(settingsLocation);
            }}
            className="block w-full rounded-md px-3 py-2 text-left text-sm text-ink hover:bg-select-fill"
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
            className="block w-full rounded-md px-3 py-2 text-left text-sm text-ink hover:bg-select-fill disabled:text-ink-faint"
          >
            {opening ? t("home.opening") : t("home.connect")}
          </button>
        </div>
      ) : null}
    </div>
  );
}

import type { ReactNode, RefObject } from "react";
import { Icon } from "./icons";
import { useTranslate } from "../i18n/context";

export type InspectorContent = { label: string; attention: boolean; body: ReactNode } | null;

const inspectorId = "inspector";

export function InspectorToggle({
  label,
  open,
  attention,
  onToggle,
  buttonRef,
}: {
  label: string;
  open: boolean;
  attention: boolean;
  onToggle: () => void;
  buttonRef?: RefObject<HTMLButtonElement | null>;
}) {
  const t = useTranslate();
  const action = t(open ? "shell.inspectorHideNamed" : "shell.inspectorShowNamed", { label });
  const name = attention ? `${action} ${t("shell.inspectorAttention")}` : action;
  return (
    <button
      ref={buttonRef}
      type="button"
      aria-label={name}
      aria-expanded={open}
      aria-controls={inspectorId}
      onClick={onToggle}
      className={`relative flex items-center gap-1.5 rounded-md border border-control-line px-2 py-1 text-ink transition-colors duration-150 hover:border-accent/50 ${
        open ? "bg-select-fill" : "bg-card hover:bg-hover"
      }`}
    >
      <Icon name="inspector" className="h-4 w-4" />
      <span className="hidden max-w-44 truncate text-xs sm:inline">{label}</span>

      {attention ? (
        <span
          aria-hidden="true"
          className="absolute -right-1 -top-1 h-2 w-2 rounded-full border border-toolbar bg-notice-ink"
        />
      ) : null}
    </button>
  );
}

// InspectorPane is the details pane. On a wide screen it is a column beside the page; narrower, it covers
// the page, and below the top bar (h-12) where there is one. Covering the page, it also covers the page's
// own toggle when there is no top bar (a PC window under 1024px), so it carries a close button of its own.
export function InspectorPane({
  label,
  underTopBar,
  onClose,
  children,
  paneRef,
}: {
  label: string;
  underTopBar: boolean;
  onClose: () => void;
  children: ReactNode;
  paneRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  return (
    <aside
      ref={paneRef}
      tabIndex={-1}
      id={inspectorId}
      aria-label={label}
      className={`fixed inset-x-0 bottom-0 z-10 overflow-y-auto bg-sidebar p-3 lg:relative lg:inset-auto lg:z-auto lg:border-l lg:border-line ${underTopBar ? "top-12" : "top-0"}`}
    >
      <div className="sticky top-0 z-[1] -mx-3 -mt-3 mb-1 flex justify-end bg-sidebar px-2 pt-2 lg:hidden">
        <button
          type="button"
          aria-label={t("shell.inspectorHideNamed", { label })}
          title={t("shell.inspectorHideNamed", { label })}
          onClick={onClose}
          className="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
        >
          <Icon name="close" className="size-4" />
        </button>
      </div>
      {children}
    </aside>
  );
}

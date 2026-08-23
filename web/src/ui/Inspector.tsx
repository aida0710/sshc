import type { ReactNode } from "react";
import { Icon } from "./icons";
import { useTranslate } from "../i18n/context";

export type InspectorContent = { label: string; attention: boolean; body: ReactNode } | null;

export const inspectorId = "inspector";

export function InspectorToggle({
  label,
  open,
  attention,
  onToggle,
}: {
  label: string;
  open: boolean;
  attention: boolean;
  onToggle: () => void;
}) {
  const t = useTranslate();
  const action = t(open ? "shell.inspectorHideNamed" : "shell.inspectorShowNamed", { label });
  const name = attention ? `${action} ${t("shell.inspectorAttention")}` : action;
  return (
    <button
      type="button"
      aria-label={name}
      aria-expanded={open}
      aria-controls={inspectorId}
      onClick={onToggle}
      className={`relative flex items-center gap-1.5 rounded-md border border-control-line px-2 py-1 text-ink ${
        open ? "bg-select-fill" : "bg-card"
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

export function InspectorPane({ label, children }: { label: string; children: ReactNode }) {
  return (
    <aside
      id={inspectorId}
      aria-label={label}
      className="fixed inset-0 z-10 overflow-y-auto bg-sidebar p-3 lg:relative lg:z-auto lg:border-l lg:border-line"
    >
      {children}
    </aside>
  );
}

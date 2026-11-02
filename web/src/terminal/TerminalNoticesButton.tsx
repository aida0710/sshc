import type { RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { NoticeSeverity } from "./statusBannerRows";

// TerminalNoticesButton stands for a phone's notices about the session (the strip and the card a wider screen shows
// at once) in its top bar; a tap shows or hides them. Its colour says whether something ended or failed.
export function TerminalNoticesButton({ severity, expanded, onToggle, buttonRef }: {
  severity: NoticeSeverity;
  expanded: boolean;
  onToggle: () => void;
  buttonRef: RefObject<HTMLButtonElement | null>;
}) {
  const t = useTranslate();
  return (
    <button
      ref={buttonRef}
      type="button"
      aria-label={t("terminal.notices")}
      title={t("terminal.notices")}
      aria-expanded={expanded}
      data-terminal-notices={severity}
      onClick={onToggle}
      className={`flex size-11 shrink-0 items-center justify-center rounded-lg hover:bg-hover focus:outline-none focus-visible:bg-hover ${expanded ? "bg-hover" : ""} ${severity === "danger" ? "text-danger" : "text-notice-ink"}`}
    >
      <Icon name="warning" className="size-5" />
    </button>
  );
}

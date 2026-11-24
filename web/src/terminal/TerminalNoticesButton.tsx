import type { RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import type { NoticeSeverity } from "./statusBannerRows";
import { IconButton } from "../ui/IconButton";

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
    <IconButton
      ref={buttonRef}
      label={t("terminal.notices")}
      size="bar"
      tone="plain"
      aria-expanded={expanded}
      data-terminal-notices={severity}
      onClick={onToggle}
      className={`focus-visible:bg-hover ${expanded ? "bg-hover" : ""} ${severity === "danger" ? "text-danger" : "text-notice-ink"}`}
    >
      <Icon name="warning" className="size-5" />
    </IconButton>
  );
}

import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { DisclosureSummary } from "./DisclosureSummary";

// MoreDetails keeps the rest of an explanation closed under a short note, so a page shows one sentence
// until someone asks for more (the design's rule for descriptions that are always on screen).
export function MoreDetails({ children, className = "" }: { children: ReactNode; className?: string }) {
  const t = useTranslate();
  return (
    <details className={`text-xs leading-[18px] text-ink-faint ${className}`}>
      <DisclosureSummary className="text-ink-muted">{t("ui.moreDetails")}</DisclosureSummary>
      <div className="mt-1.5 pl-5">{children}</div>
    </details>
  );
}

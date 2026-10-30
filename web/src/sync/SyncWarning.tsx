import { useId, useState } from "react";
import { useTranslate } from "../i18n/context";

// SyncWarning says in one line what the sync sends and that it is encrypted first. The whole of it (what is
// left out, who can read what) opens under the line: it matters, but not on every visit to the page.
export function SyncWarning() {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const detailId = useId();
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2 text-[13px] leading-5 text-ink-muted">
        <p className="min-w-0 flex-1">{t("sync.warningSummary")}</p>
        <button type="button" aria-expanded={open} aria-controls={detailId} onClick={() => setOpen((value) => !value)}
          className="shrink-0 rounded-md text-[13px] text-accent hover:underline">
          {t(open ? "sync.warningLess" : "sync.warningMore")}
        </button>
      </div>
      {open ? <p id={detailId} className="rounded-[9px] bg-well px-3 py-2.5 text-xs leading-[18px] text-ink-muted">{t("sync.warning")}</p> : null}
    </div>
  );
}

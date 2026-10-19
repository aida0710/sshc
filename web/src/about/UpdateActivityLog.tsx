import { useLanguage } from "../i18n/context";
import { formatTimeOfDay } from "../ui/format";
import type { UpdateActivityEntry } from "./useUpdateActivity";

// UpdateActivityLog lists, with the time of each, what a check or an update did.
// Nothing is drawn until the first step, so the page has no empty box.
export function UpdateActivityLog({ entries }: { entries: UpdateActivityEntry[] }) {
  const { t, locale } = useLanguage();
  if (entries.length === 0) return null;
  // The log role sits on a wrapper: on the list itself it would replace the list semantics its items need.
  return (
    <div role="log" aria-label={t("about.activity")} className="rounded border border-line px-3 py-2 text-xs">
      <ol className="grid gap-1">
        {entries.map((entry) => (
          <li key={entry.id} className={`flex gap-3 ${entry.tone === "danger" ? "text-danger" : "text-ink-muted"}`}>
            <time dateTime={entry.at.toISOString()} className="shrink-0 tabular-nums">{formatTimeOfDay(entry.at, locale)}</time>
            <span className="min-w-0 break-words">{t(entry.message, entry.values)}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

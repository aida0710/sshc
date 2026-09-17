import { useTranslate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { hintText, sectionHeading } from "../ui/form";
import { PanelState } from "../ui/PanelState";
import { Button } from "../ui/surface";
import { formatBytes } from "./SyncResultCard";
import type { BucketStatusState } from "./useSyncRemoteState";

// History entries shown before "show all" is asked for.
const collapsedHistoryCount = 5;

// What the bucket itself holds: the live snapshot, whether this engine is
// on it, and the retained history objects.
export function SyncBucketStateSection({ bucketState, locale, busy, historyExpanded, onToggleHistory, onRefresh }: {
  bucketState: BucketStatusState;
  locale: Locale;
  busy: boolean;
  historyExpanded: boolean;
  onToggleHistory: () => void;
  onRefresh: () => void;
}) {
  const t = useTranslate();
  const historyItems = bucketState.phase === "ready" ? bucketState.value.history : [];
  const visibleHistory = historyExpanded ? historyItems : historyItems.slice(0, collapsedHistoryCount);
  return (
    <section
      aria-labelledby="sync-bucket-state-heading"
      className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4 lg:col-span-2"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 id="sync-bucket-state-heading" className={sectionHeading}>
            {t("sync.bucketStateHeading")}
          </h4>
          <p className={`mt-1 ${hintText}`}>
            {t("sync.bucketStateHint")}
          </p>
        </div>
        <Button
          disabled={busy || bucketState.phase === "loading"}
          onClick={onRefresh}
        >
          {t("sync.bucketRefresh")}
        </Button>
      </div>

      {bucketState.phase === "idle" ? (
        <PanelState tone="empty" title={t("sync.bucketNotConfigured")} />
      ) : bucketState.phase === "loading" ? (
        <PanelState tone="loading" title={t("sync.bucketLoading")} />
      ) : bucketState.phase === "error" ? (
        <PanelState tone="failed" title={bucketState.message} action={<Button onClick={onRefresh}>{t("sync.bucketRefresh")}</Button>} />
      ) : (
        <div className="grid gap-3 border-t border-line pt-3 lg:grid-cols-2">
          <div className="rounded border border-line bg-surface p-3">
            <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
              {t("sync.bucketLive")}
            </p>
            {bucketState.value.live === undefined ? (
              <p className="mt-2 text-sm text-ink-muted">
                {t("sync.bucketLiveEmpty")}
              </p>
            ) : (
              <div className="mt-2 flex flex-col gap-1">
                <p className={hintText}>
                  {t("sync.bucketObjectMeta", {
                    size: formatBytes(
                      bucketState.value.live.size,
                      locale,
                    ),
                    at: bucketState.value.live.lastModified ?? "—",
                  })}
                </p>
                <p
                  className={`text-sm font-medium ${bucketState.value.localIsLive ? "text-success" : "text-notice-ink"}`}
                >
                  {t(
                    bucketState.value.localIsLive
                      ? "sync.bucketLocalCurrent"
                      : "sync.bucketLocalBehind",
                  )}
                </p>
                <details className="mt-1 text-xs text-ink-muted">
                  <summary className="cursor-pointer">
                    {t("sync.bucketObjectName")}
                  </summary>
                  <p className="mt-1 break-all font-mono text-ink">
                    {bucketState.value.live.key}
                  </p>
                </details>
              </div>
            )}
          </div>

          <div className="rounded border border-line bg-surface p-3">
            <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
              {t("sync.bucketHistory", {
                count: bucketState.value.history.length,
              })}
            </p>
            {bucketState.value.historyTruncated ? (
              <p className="mt-2 text-xs text-notice-ink">
                {t("sync.bucketHistoryTruncated")}
              </p>
            ) : null}
            {bucketState.value.history.length === 0 ? (
              <p className="mt-2 text-sm text-ink-muted">
                {t("sync.bucketHistoryEmpty")}
              </p>
            ) : (
              <div className="mt-2 flex flex-col gap-2">
                <p className={hintText}>
                  {t("sync.bucketHistoryShowing", {
                    shown: visibleHistory.length,
                    count: bucketState.value.history.length,
                  })}
                </p>
                <ul className="max-h-64 space-y-2 overflow-auto pr-1">
                  {visibleHistory.map((item) => (
                    <li
                      key={item.key}
                      className="rounded border border-hairline px-2 py-1.5"
                    >
                      <p className={hintText}>
                        {t("sync.bucketObjectMeta", {
                          size: formatBytes(item.size, locale),
                          at: item.lastModified ?? "—",
                        })}
                      </p>
                      <details className="mt-1 text-xs text-ink-muted">
                        <summary className="cursor-pointer">
                          {t("sync.bucketObjectName")}
                        </summary>
                        <p className="mt-1 break-all font-mono text-ink">
                          {item.key}
                        </p>
                      </details>
                    </li>
                  ))}
                </ul>
                {bucketState.value.history.length <= collapsedHistoryCount ? null : (
                  <Button
                    onClick={onToggleHistory}
                    className="self-start"
                  >
                    {t(
                      historyExpanded
                        ? "sync.bucketHistoryCollapse"
                        : "sync.bucketHistoryExpand",
                    )}
                  </Button>
                )}
              </div>
            )}
          </div>

          <p className={`lg:col-span-2 ${hintText}`}>
            {t("sync.bucketCheckedAt", {
              at: bucketState.value.checkedAt,
            })}
          </p>
        </div>
      )}
    </section>
  );
}

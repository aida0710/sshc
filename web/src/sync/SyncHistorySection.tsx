import type { SyncDirection, SyncHistoryDiff } from "../api/integrations";
import type { Translate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { hintText, sectionHeading } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { formatBytes } from "./SyncResultCard";
import type { HistoryState } from "./useSyncRemoteState";

type SyncHistorySectionProps = {
  busy: boolean;
  direction: SyncDirection;
  historyDiff: SyncHistoryDiff | null;
  historyState: HistoryState;
  keyConfigured: boolean;
  locale: Locale;
  selectedKey: string | null;
  t: Translate;
  onPreview: (key: string) => void;
  onRefresh: () => void;
  onSelect: (key: string) => void;
};

export function SyncHistorySection({
  busy,
  direction,
  historyDiff,
  historyState,
  keyConfigured,
  locale,
  selectedKey,
  t,
  onPreview,
  onRefresh,
  onSelect,
}: SyncHistorySectionProps) {
  const selected =
    historyState.phase === "ready" && selectedKey !== null
      ? historyState.value.revisions.find(
          (revision) => revision.key === selectedKey,
        )
      : undefined;

  return (
    <section
      aria-labelledby="sync-history-heading"
      className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4 lg:col-span-2"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 id="sync-history-heading" className={sectionHeading}>
            {t("sync.historyHeading")}
          </h4>
          <p className={`mt-1 ${hintText}`}>{t("sync.historyHint")}</p>
        </div>
        <Button
          disabled={busy || !keyConfigured || historyState.phase === "loading"}
          onClick={onRefresh}
        >
          {t("sync.historyRefresh")}
        </Button>
      </div>

      {historyState.phase === "idle" ? (
        <p className={hintText}>{t("sync.historyNeedsKey")}</p>
      ) : historyState.phase === "loading" ? (
        <p role="status" className={hintText}>
          {t("sync.historyLoading")}
        </p>
      ) : historyState.phase === "error" ? (
        <Notice tone="danger">{historyState.message}</Notice>
      ) : (
        <div className="border-t border-line pt-3">
          <p className={hintText}>
            {t("sync.historySummary", {
              count: historyState.value.revisions.length,
              size: formatBytes(historyState.value.downloadedBytes, locale),
            })}
          </p>
          {historyState.value.historyTruncated ||
          historyState.value.downloadTruncated ? (
            <p className="mt-1 text-xs text-notice-ink">
              {t("sync.historyTruncated")}
            </p>
          ) : null}
          {historyState.value.skipped > 0 ? (
            <p className="mt-1 text-xs text-notice-ink">
              {t("sync.historySkipped", {
                count: historyState.value.skipped,
              })}
            </p>
          ) : null}

          <div className="mt-3 grid gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
            <ol
              className="max-h-80 space-y-2 overflow-auto pr-1"
              aria-label={t("sync.historyTimeline")}
            >
              {historyState.value.revisions.map((revision) => (
                <li
                  key={revision.key}
                  className="relative border-l-2 border-line pl-4"
                >
                  <span className="absolute -left-[5px] top-4 h-2 w-2 rounded-full bg-accent" />
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onSelect(revision.key)}
                    className={`w-full rounded border px-3 py-2 text-left ${
                      selectedKey === revision.key
                        ? "border-accent bg-select-fill"
                        : "border-hairline bg-surface hover:border-line"
                    }`}
                  >
                    <span className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-mono text-xs font-semibold text-ink">
                        {revision.revision.slice(0, 12)}
                      </span>
                      <span className="rounded-full bg-toolbar px-2 py-0.5 text-[11px] font-medium text-ink-muted">
                        {t(
                          `sync.historyRelation.${revision.relation}` as MessageKey,
                        )}
                      </span>
                    </span>
                    <span className="mt-1 block text-xs text-ink-muted">
                      {t("sync.historyRevisionMeta", {
                        at: revision.createdAt,
                        count: revision.fileCount,
                        origin: revision.origin.slice(0, 8),
                      })}
                    </span>
                    <span className="mt-1 block text-sm font-medium text-ink">
                      {revision.message}
                    </span>
                    {revision.parentRevision === undefined ? null : (
                      <span className="mt-1 block font-mono text-[11px] text-ink-muted">
                        {t("sync.historyParent", {
                          revision: revision.parentRevision.slice(0, 12),
                        })}
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ol>

            <div className="min-w-0 rounded border border-line bg-surface p-3">
              {selected === undefined ? (
                <p className={hintText}>{t("sync.historySelect")}</p>
              ) : (
                <div className="flex flex-col gap-3">
                  <div>
                    <p className="text-xs font-medium uppercase tracking-wide text-ink-muted">
                      {t("sync.historySelected")}
                    </p>
                    <p className="mt-1 break-all font-mono text-xs text-ink">
                      {selected.key}
                    </p>
                  </div>
                  {historyDiff === null ? (
                    <p role="status" className={hintText}>
                      {busy
                        ? t("sync.historyDiffLoading")
                        : t("sync.historyDiffEmpty")}
                    </p>
                  ) : (
                    <div className="grid gap-2 text-xs sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
                      {(
                        [
                          ["added", historyDiff.added, "text-success"],
                          ["modified", historyDiff.modified, "text-notice-ink"],
                          ["removed", historyDiff.removed, "text-danger"],
                        ] as const
                      ).map(([kind, paths, tone]) => (
                        <div
                          key={kind}
                          className="min-w-0 rounded bg-toolbar p-2"
                        >
                          <p className={`font-medium ${tone}`}>
                            {t(`sync.historyDiff.${kind}` as MessageKey, {
                              count: paths.length,
                            })}
                          </p>
                          <ul className="mt-1 max-h-24 space-y-0.5 overflow-auto font-mono text-[11px] text-ink-muted">
                            {paths.map((path) => (
                              <li key={path} className="break-all">
                                {path}
                              </li>
                            ))}
                          </ul>
                        </div>
                      ))}
                    </div>
                  )}
                  <p className={hintText}>{t("sync.historyRestoreHint")}</p>
                  <Button
                    kind="primary"
                    disabled={busy || direction === "push"}
                    onClick={() => onPreview(selected.key)}
                    className="self-start"
                  >
                    {t("sync.historyRestorePreview")}
                  </Button>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </section>
  );
}

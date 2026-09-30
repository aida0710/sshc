import type { SyncHistoryDiff, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { hintText } from "../ui/form";
import { describeLastSync } from "./describeLastSync";
import { SyncBucketStateSection } from "./SyncBucketStateSection";
import { SyncHistorySection } from "./SyncHistorySection";
import type { BucketStatusState, HistoryState } from "./useSyncRemoteState";

// SyncDetailsSection は、たたんで置く詳細である。バケットの状態と、暗号化した
// 履歴を並べる。
export function SyncDetailsSection({
  status,
  locale,
  busy,
  bucketState,
  bucketHistoryExpanded,
  onToggleBucketHistory,
  onRefreshBucket,
  historyState,
  historyDiff,
  selectedHistoryKey,
  onRefreshHistory,
  onSelectHistory,
  onPreviewHistory,
}: {
  status: SyncStatus;
  locale: Locale;
  busy: boolean;
  bucketState: BucketStatusState;
  bucketHistoryExpanded: boolean;
  onToggleBucketHistory: () => void;
  onRefreshBucket: () => void;
  historyState: HistoryState;
  historyDiff: SyncHistoryDiff | null;
  selectedHistoryKey: string | null;
  onRefreshHistory: () => void;
  onSelectHistory: (key: string) => void;
  onPreviewHistory: (key: string) => void;
}) {
  const t = useTranslate();
  return (
    <details className="overflow-hidden rounded-md border border-control-line bg-card">
      <summary className="flex cursor-pointer list-none flex-wrap items-center justify-between gap-3 bg-toolbar px-4 py-3 marker:hidden hover:bg-select-fill">
        <span className="flex items-center gap-3 text-sm font-medium text-ink">
          <DisclosureChevron className="size-4 text-ink-muted" />
          {t("sync.detailsHeading")}
        </span>
        <span className={hintText}>{describeLastSync(t, status, locale)}</span>
      </summary>
      <div className="grid gap-4 border-t border-line bg-surface-subtle p-4 lg:grid-cols-2">
        <SyncBucketStateSection
          bucketState={bucketState}
          locale={locale}
          busy={busy}
          historyExpanded={bucketHistoryExpanded}
          onToggleHistory={onToggleBucketHistory}
          onRefresh={onRefreshBucket}
        />
        <SyncHistorySection
          busy={busy}
          direction={status.direction}
          historyDiff={historyDiff}
          historyState={historyState}
          keyConfigured={status.keyConfigured}
          locale={locale}
          selectedKey={selectedHistoryKey}
          t={t}
          onPreview={onPreviewHistory}
          onRefresh={onRefreshHistory}
          onSelect={onSelectHistory}
        />
      </div>
    </details>
  );
}

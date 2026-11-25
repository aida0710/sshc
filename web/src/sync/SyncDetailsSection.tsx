import type { SyncHistoryDiff, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { DisclosureSummary } from "../ui/DisclosureSummary";
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
  onPreviewHistory: (key: string) => Promise<void>;
}) {
  const t = useTranslate();
  return (
    <details className="border-t border-line pt-4">
      <DisclosureSummary className="text-sm font-semibold text-ink">
        {t("sync.detailsHeading")}
      </DisclosureSummary>
      <div className="grid gap-4 pt-4 lg:grid-cols-2">
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

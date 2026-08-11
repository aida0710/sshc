import type {
  PullResponse,
  PushResult,
  SnapshotSummary,
  SyncOperation,
} from "../api/integrations";
import { useLanguage } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { Card } from "../ui/surface";

export type SyncResultView =
  | { kind: "push"; result: PushResult }
  | { kind: "preview"; result: PullResponse }
  | { kind: "apply"; result: PullResponse }
  | { kind: "previous"; operation: SyncOperation };

export function formatBytes(bytes: number, locale: Locale): string {
  const units = ["B", "kB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit++;
  }
  const formatted = new Intl.NumberFormat(locale, {
    maximumFractionDigits: unit === 0 ? 0 : 1,
  }).format(value);
  return `${formatted} ${units[unit]}`;
}

function formatMoment(value: string, locale: Locale): string {
  const moment = new Date(value);
  if (Number.isNaN(moment.getTime())) return value;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(moment);
}

function SummaryLines({ summary }: { summary: SnapshotSummary }) {
  const { locale, t } = useLanguage();
  return (
    <>
      <p className="font-medium text-ink">
        {t("sync.result.filesSource", {
          count: new Intl.NumberFormat(locale).format(summary.fileCount),
          size: formatBytes(summary.sourceBytes, locale),
        })}
      </p>
      <p className="text-sm text-ink-muted">
        {t("sync.result.encrypted", { size: formatBytes(summary.snapshotBytes, locale) })}
      </p>
    </>
  );
}

export function SyncResultCard({ view }: { view: SyncResultView }) {
  const { locale, t } = useLanguage();
  let summary: SnapshotSummary;
  let completedAt: string;
  let displayKind: "push" | "preview" | "apply";
  let objectCount = 0;
  let uploadedBytes = 0;
  let downloadedBytes = 0;
  let written = 0;
  let removed = 0;
  let conflicts = 0;

  if (view.kind === "previous") {
    summary = view.operation.summary;
    completedAt = view.operation.completedAt;
    displayKind = view.operation.kind;
    objectCount = view.operation.objectCount ?? 0;
    uploadedBytes = view.operation.uploadedBytes ?? 0;
    downloadedBytes = view.operation.downloadedBytes ?? 0;
    written = view.operation.written ?? 0;
    removed = view.operation.removed ?? 0;
  } else if (view.kind === "push") {
    summary = view.result.summary;
    completedAt = view.result.completedAt;
    displayKind = "push";
    objectCount = view.result.objectCount;
    uploadedBytes = view.result.uploadedBytes;
  } else {
    summary = view.result.summary;
    completedAt = view.result.completedAt;
    displayKind = view.kind;
    downloadedBytes = view.result.downloadedBytes;
    written = view.result.written.length;
    removed = view.result.removed.length;
    conflicts = view.result.conflicts.length;
  }

  const heading = view.kind === "previous"
    ? t("sync.result.previousTitle")
    : t(
        displayKind === "push"
          ? "sync.result.pushTitle"
          : displayKind === "preview"
            ? "sync.result.previewTitle"
            : "sync.result.applyTitle",
      );

  return (
    <section aria-label={heading}>
      <Card padded>
        <h4 className="text-sm font-semibold text-ink">{heading}</h4>
        {displayKind === "push" ? (
          <>
            <SummaryLines summary={summary} />
            <p className="text-sm text-ink-muted">
              {t("sync.result.uploaded", {
                size: formatBytes(uploadedBytes, locale),
                count: new Intl.NumberFormat(locale).format(objectCount),
              })}
            </p>
            <p className="text-xs text-ink-muted">
              {t("sync.result.created", { at: formatMoment(summary.createdAt, locale) })}
            </p>
          </>
        ) : displayKind === "preview" ? (
          <>
            <p className="font-medium text-ink">
              {t("sync.result.previewDownload", {
                downloaded: formatBytes(downloadedBytes, locale),
                source: formatBytes(summary.sourceBytes, locale),
              })}
            </p>
            <p className="text-sm text-ink-muted">
              {t("sync.result.snapshotAt", { at: formatMoment(summary.createdAt, locale) })}
            </p>
            <p className="text-sm text-ink-muted">
              {t("sync.result.changes", { written, removed, conflicts })}
            </p>
          </>
        ) : (
          <>
            <p className="font-medium text-ink">
              {t("sync.result.appliedSnapshot", { at: formatMoment(summary.createdAt, locale) })}
            </p>
            <p className="text-sm text-ink-muted">
              {t("sync.result.changes", { written, removed, conflicts })}
            </p>
            <p className="text-sm text-ink-muted">
              {t("sync.result.applyDownload", { size: formatBytes(downloadedBytes, locale) })}
            </p>
          </>
        )}
        <p className="text-xs text-ink-muted">
          {t("sync.result.completed", { at: formatMoment(completedAt, locale) })}
        </p>
      </Card>
    </section>
  );
}

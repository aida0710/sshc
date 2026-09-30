import type { SyncStatus } from "../api/sync";
import type { Translate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { formatOptionalDateTime } from "../ui/format";

// The overview card and the collapsed details both say when this machine last
// synced; building the sentence in one place keeps the two lines identical.
export function describeLastSync(
  t: Translate,
  status: Pick<SyncStatus, "synced" | "lastSyncedAt" | "fileCount">,
  locale: Locale,
): string {
  if (!status.synced) return t("sync.neverSynced");
  return t("sync.lastSynced", {
    at: formatOptionalDateTime(status.lastSyncedAt, locale),
    count: status.fileCount ?? 0,
  });
}

import type { SyncStatus } from "../api/sync";
import type { Translate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { formatOptionalDateTime } from "../ui/format";

// The line under the overview card's heading: when this machine last synced.
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

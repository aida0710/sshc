// The document a set-up machine exports so that another can join the same sync target: the bucket settings and
// credentials sealed with the encryption key (internal/remotesync/settings_export.go). The page never opens it;
// it only tells such a document from something else before handing it to the engine.

export const syncSetupExportFileName = "sshc-sync-settings.json";

// The engine refuses a longer document (remotesync.MaxSettingsExportLength).
const maxSyncSetupExportLength = 16 * 1024;

export function looksLikeSyncSetupExport(text: string): boolean {
  if (text.length > maxSyncSetupExportLength) return false;
  try {
    const parsed: unknown = JSON.parse(text);
    return (
      typeof parsed === "object" &&
      parsed !== null &&
      typeof (parsed as { sshcSyncSettings?: unknown }).sshcSyncSettings === "number" &&
      typeof (parsed as { sealed?: unknown }).sealed === "string"
    );
  } catch {
    return false;
  }
}

import { useCallback } from "react";
import { useTranslate } from "../i18n/context";
import { failureCode } from "../api/client";
import { formatBytes } from "../ui/format";
import { useLoad } from "../ui/useLoad";
import { remoteMetadataApi } from "./remoteMetadataApi";

// The engine sends 64-bit counts as decimal strings. Number loses precision
// only past 2^53 bytes (8 PiB), and by far less than the one decimal shown.
function formatFilesystemBytes(bytes: string): string {
  return formatBytes(Number(bytes));
}

// The capacity request has its own state: an unsupported extension never fails the listing.
export function SFTPFilesystemSpace({ alias, path, refreshKey }: { alias: string; path: string; refreshKey: unknown }) {
  const t = useTranslate();
  // refreshKey reads the space again after the listing changes; it is not passed to the request.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const space = useLoad(useCallback(() => remoteMetadataApi.filesystemSpace(alias, path), [alias, path, refreshKey]));
  if (space.state === "loading") return null;
  const unsupported = space.state === "failed" && failureCode(space.error) === "sftp_unsupported_operation";
  return <p className="shrink-0 px-2 py-1 text-xs text-ink-muted" role="status">{space.state === "loaded"
    ? t("sftp.filesystemSpace", { available: formatFilesystemBytes(space.value.availableBytes), total: formatFilesystemBytes(space.value.totalBytes) })
    : t(unsupported ? "sftp.spaceUnsupported" : "sftp.spaceUnavailable")}</p>;
}

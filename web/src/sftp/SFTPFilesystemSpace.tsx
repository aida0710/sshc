import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { failureCode } from "../api/client";
import { remoteMetadataApi, type FilesystemSpace } from "./remoteMetadataApi";

type SpaceState = { alias: string; path: string } & (
  | { kind: "loaded"; space: FilesystemSpace }
  | { kind: "unsupported" | "unavailable" }
);

// The capacity request has its own state: an unsupported extension never fails the listing.
export function SFTPFilesystemSpace({ alias, path, refreshKey }: { alias: string; path: string; refreshKey: unknown }) {
  const t = useTranslate();
  const [state, setState] = useState<SpaceState | null>(null);
  useEffect(() => {
    let active = true;
    void remoteMetadataApi.filesystemSpace(alias, path).then(
      (space) => { if (active) setState({ kind: "loaded", alias, path, space }); },
      (error: unknown) => { if (active) setState({ kind: failureCode(error) === "sftp_unsupported_operation" ? "unsupported" : "unavailable", alias, path }); },
    );
    return () => { active = false; };
  }, [alias, path, refreshKey]);
  if (state === null || state.alias !== alias || state.path !== path) return null;
  return <p className="shrink-0 px-2 py-1 text-xs text-ink-muted" role="status">{state.kind === "loaded"
    ? t("sftp.filesystemSpace", { available: BigInt(state.space.availableBytes).toLocaleString(), total: BigInt(state.space.totalBytes).toLocaleString() })
    : t(state.kind === "unsupported" ? "sftp.spaceUnsupported" : "sftp.spaceUnavailable")}</p>;
}

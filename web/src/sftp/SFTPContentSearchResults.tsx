import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { formatBytes } from "../ui/format";
import type { RemoteSearchResult } from "./api";

type ContentMatch = NonNullable<RemoteSearchResult["matches"]>[number];

const omissionLabels: Record<string, MessageKey> = {
  symlink: "sftp.search.skip.symlink", unsupported: "sftp.search.skip.unsupported",
  binary: "sftp.search.skip.binary", file_size: "sftp.search.skip.fileSize",
  unreadable: "sftp.search.skip.unreadable", changed: "sftp.search.skip.changed",
  byte_limit: "sftp.search.skip.bytes", result_limit: "sftp.search.skip.results",
  entry_limit: "sftp.search.skip.entries", depth_limit: "sftp.search.skip.depth",
};

export function SFTPContentSearchResults({ search, disabled, onOpen }: {
  search: RemoteSearchResult;
  disabled: boolean;
  onOpen: (match: ContentMatch) => void;
}) {
  const t = useTranslate();
  const matches = search.matches ?? [];
  return <div className="text-sm">
    <p className="px-3 py-2 text-xs text-ink-muted">{t("sftp.search.bytesRead", { bytes: formatBytes(search.bytesRead ?? 0) })}</p>
    {(search.omissions ?? []).length === 0 ? null : <ul aria-label={t("sftp.search.omissions")} className="border-b border-line/50 px-3 pb-2 text-xs text-ink-muted">
      {search.omissions?.map(({ reason, count }) => <li key={reason}>{t(omissionLabels[reason] ?? "sftp.search.skip.unsupported", { count })}</li>)}
    </ul>}
    {matches.length === 0 ? <p className="p-3 text-ink-muted">{t(search.truncated ? "sftp.search.noContentMatchesPartial" : "sftp.search.noContentMatches")}</p> : <ul>
      {matches.map((match) => <li key={`${match.entry.path}:${match.line}`} className="border-b border-line/40">
        <button type="button" disabled={disabled} aria-label={t("sftp.search.openLine", { path: match.entry.path, line: match.line })} onClick={() => onOpen(match)} className="block w-full px-3 py-2 text-left hover:bg-hover disabled:text-ink-faint">
          <span className="block break-all font-mono text-xs text-accent">{match.entry.path}:{match.line}</span>
          <span className="mt-1 block break-all font-mono text-xs text-ink-muted">{match.snippet}</span>
        </button>
      </li>)}
    </ul>}
  </div>;
}

import { useLanguage, useTranslate } from "../i18n/context";
import { formatDateTime } from "../ui/format";
import type { RemoteEntry } from "./api";
import { EntryThumbnail } from "./EntryThumbnail";
import { EntryIcon, EntryName, entrySize } from "./sftpEntryLook";
import { useEntryRowProps } from "./useEntryRowProps";
import type { SFTPEntryListModel } from "./useSFTPEntryList";

// As tall as the row's two lines of text, so that a folder with pictures fits as many rows on the screen as any other.
const phoneThumbnailBox = "size-[38px] rounded";

// What a row says under its name: the size and the date come first; the permissions and the owner are in the
// details. A search result says first which folder it is in.
function EntryFacts({ entry, entryContext }: { entry: RemoteEntry; entryContext: ((entry: RemoteEntry) => string) | undefined }) {
  const { locale } = useLanguage();
  return (
    <span className="mt-0.5 flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs leading-4 text-ink-muted">
      {entryContext === undefined ? null : <span title={entryContext(entry)} className="max-w-full truncate font-mono">{entryContext(entry)}</span>}
      <span className="whitespace-nowrap">
        {entrySize(entry)}{" · "}
        <time title={new Date(entry.modifiedAt).toLocaleString(locale)} dateTime={entry.modifiedAt}>{formatDateTime(entry.modifiedAt, locale)}</time>
      </span>
    </span>
  );
}

// The rows on a phone: two lines, with targets of at least 44px. A tap opens a row. A long press starts choosing rows,
// and only while some are chosen do the rows have checkboxes and a tap choose. The folder's way up is in the toolbar,
// so that it does not take a file's row.
export function SFTPEntryPhoneList({ model, entries, busy, entryContext, thumbnailAlias }: {
  model: SFTPEntryListModel;
  entries: RemoteEntry[];
  busy: boolean;
  entryContext: ((entry: RemoteEntry) => string) | undefined;
  thumbnailAlias: string | null;
}) {
  const t = useTranslate();
  const { selectedPaths, rowContextMenu } = model;
  const { entryCheckboxProps, entryButtonProps } = useEntryRowProps({ model, busy });
  const choosing = selectedPaths.size > 0;
  return (
    <ul aria-label={t("sftp.entries")} className="divide-y divide-line/40">
      {entries.map((entry) => (
        <li
          key={entry.path}
          data-row-key={entry.path}
          className={`flex items-center transition-colors ${selectedPaths.has(entry.path) ? "bg-select-fill/75" : ""}`}
          onContextMenu={(event) => rowContextMenu(event, entry)}
        >
          {choosing ? (
            <label className="flex min-h-11 w-11 shrink-0 self-stretch items-center justify-center">
              <input type="checkbox" {...entryCheckboxProps(entry)} className="size-5 accent-accent" />
            </label>
          ) : null}
          <button
            type="button"
            {...entryButtonProps(entry)}
            className="flex min-h-14! min-w-0 flex-1 touch-pan-y select-none items-center gap-2.5 px-2.5 py-1.5 text-left hover:bg-hover active:bg-select-fill"
          >
            {thumbnailAlias === null
              ? <EntryIcon entry={entry} className="size-5 shrink-0" folderColour="text-ink-muted" />
              : <EntryThumbnail alias={thumbnailAlias} entry={entry} boxClassName={phoneThumbnailBox} iconClassName="size-5" folderColour="text-ink-muted" />}
            <span className="min-w-0 flex-1">
              <span title={entry.type === "symlink" ? `${entry.name} → ${entry.linkTarget ?? ""}` : entry.name} className="block truncate font-mono text-base font-medium leading-5 text-ink"><EntryName entry={entry} /></span>
              <EntryFacts entry={entry} entryContext={entryContext} />
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

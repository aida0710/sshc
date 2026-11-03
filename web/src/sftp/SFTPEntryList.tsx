import type { DragEvent as ReactDragEvent } from "react";
import type { RemoteEntry } from "./api";
import { SFTPEntryGrid } from "./SFTPEntryGrid";
import { SFTPEntryPhoneList } from "./SFTPEntryPhoneList";
import { SFTPEntryTable } from "./SFTPEntryTable";
import { sftpSortColumns, type SFTPSort, type SFTPSortState } from "./sftpEntrySort";
import type { SFTPEntryView } from "./sftpPanes";
import type { SFTPEntryListModel } from "./useSFTPEntryList";

// The rows of a folder: a table where a pointer can reach its columns, a two-line list on a phone, or a grid of
// pictures on either. All read the same model.
export function SFTPEntryList({
  model,
  entries,
  sort,
  onSort,
  mobileInteraction,
  compact = false,
  showOwnership = true,
  busy,
  locked = false,
  parentRowVisible,
  draggable = () => false,
  onDragStart,
  entryContext,
  view = "list",
  sortColumns = sftpSortColumns,
  thumbnailAlias = null,
}: {
  model: SFTPEntryListModel;
  entries: RemoteEntry[];
  sort: SFTPSortState;
  onSort: (key: SFTPSort) => void;
  mobileInteraction: boolean;
  compact?: boolean;
  showOwnership?: boolean;
  busy: boolean;
  locked?: boolean;
  parentRowVisible: boolean;
  draggable?: (entry: RemoteEntry) => boolean;
  onDragStart?: (event: ReactDragEvent<HTMLElement>, entry: RemoteEntry) => void;
  // Where an entry lives when the rows are not one directory, such as search
  // results. Shown under the name instead of the mode.
  entryContext?: ((entry: RemoteEntry) => string) | undefined;
  view?: SFTPEntryView;
  sortColumns?: readonly SFTPSort[];
  // The host to ask for thumbnails, or null while this list draws none.
  thumbnailAlias?: string | null;
}) {
  if (view === "grid") {
    return (
      <SFTPEntryGrid
        model={model}
        entries={entries}
        sort={sort}
        sortColumns={sortColumns}
        onSort={onSort}
        mobileInteraction={mobileInteraction}
        busy={busy}
        locked={locked}
        parentRowVisible={parentRowVisible}
        draggable={draggable}
        onDragStart={onDragStart}
        entryContext={entryContext}
        thumbnailAlias={thumbnailAlias}
      />
    );
  }
  if (mobileInteraction) return <SFTPEntryPhoneList model={model} entries={entries} busy={busy} entryContext={entryContext} thumbnailAlias={thumbnailAlias} />;
  return (
    <SFTPEntryTable
      model={model}
      entries={entries}
      sort={sort}
      onSort={onSort}
      compact={compact}
      showOwnership={showOwnership}
      busy={busy}
      locked={locked}
      parentRowVisible={parentRowVisible}
      draggable={draggable}
      onDragStart={onDragStart}
      entryContext={entryContext}
      thumbnailAlias={thumbnailAlias}
    />
  );
}

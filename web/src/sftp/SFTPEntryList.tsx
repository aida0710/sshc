import type { DragEvent as ReactDragEvent } from "react";
import type { RemoteEntry } from "./api";
import { SFTPEntryPhoneList } from "./SFTPEntryPhoneList";
import { SFTPEntryTable } from "./SFTPEntryTable";
import type { SFTPSort, SFTPSortState } from "./sftpEntrySort";
import type { SFTPEntryListModel } from "./useSFTPEntryList";

// The rows of a folder: a table where a pointer can reach its columns, and a two-line list on a phone. Both read the
// same model.
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
}) {
  if (mobileInteraction) return <SFTPEntryPhoneList model={model} entries={entries} busy={busy} entryContext={entryContext} />;
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
    />
  );
}

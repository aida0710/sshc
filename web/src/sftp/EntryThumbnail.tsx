import type { RemoteEntry } from "./api";
import { EntryIcon } from "./sftpEntryLook";
import { useEntryThumbnail } from "./useEntryThumbnail";

// EntryThumbnail draws an entry in a box of a fixed size: the file's icon, replaced by its picture once that has been
// made. The box keeps its size either way, so a row does not move when the picture arrives, and a file that has no
// picture simply keeps its icon. The name is the row's own label, so the picture says nothing to a screen reader, and
// it takes no pointer events: a long press and a drag belong to the row, not to the browser's handling of an image.
export function EntryThumbnail({ alias, entry, boxClassName, iconClassName, folderColour }: {
  alias: string | null;
  entry: RemoteEntry;
  boxClassName: string;
  iconClassName: string;
  folderColour?: string;
}) {
  const { boxRef, url } = useEntryThumbnail(alias, entry);
  return (
    <span ref={boxRef} className={`flex shrink-0 items-center justify-center overflow-hidden ${boxClassName}`}>
      {url === null
        ? <EntryIcon entry={entry} className={iconClassName} {...(folderColour === undefined ? {} : { folderColour })} />
        : <img src={url} alt="" draggable={false} className="pointer-events-none size-full object-cover" />}
    </span>
  );
}

import type { RemoteEntry } from "./api";

// What the engine can shrink (internal/sftp/thumbnail_image.go). Asking for anything else would only read the start of
// the file to refuse it.
const thumbnailExtensions = /\.(?:png|jpe?g|gif)$/iu;

// maxThumbnailedFilesInFolder is the most pictures a folder may hold and still get thumbnails. Rows are made as they
// come into view, but scrolling through thousands of photos on a phone would keep costing data all the way down.
export const maxThumbnailedFilesInFolder = 1000;

// Only a regular file has a thumbnail. A symlink's listed size and revision are the link's own, not the picture's.
export function hasThumbnail(entry: Pick<RemoteEntry, "type" | "name">): boolean {
  return entry.type === "file" && thumbnailExtensions.test(entry.name);
}

// entriesShowThumbnails says whether a list draws thumbnails at all. A list with no picture keeps its small icons, so
// the names keep the width a folder of text files has always had.
export function entriesShowThumbnails(entries: readonly Pick<RemoteEntry, "type" | "name">[]): boolean {
  const pictures = entries.filter(hasThumbnail).length;
  return pictures > 0 && pictures <= maxThumbnailedFilesInFolder;
}

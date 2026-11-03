import { useTranslate } from "../i18n/context";
import { formatBytes } from "../ui/format";
import { Icon } from "../ui/icons";
import type { RemoteEntry } from "./api";
import { entryKind } from "./entryKind";
import { fileKindIconColours, fileKindIcons, fileKindLabelKeys, fileKindOf } from "./fileKind";
import { entryTypeLabelKeys } from "./sftpMessageKeys";

// How one entry is drawn, whichever list it is in.

// The list shows sizes in units people read at a glance; the exact byte
// count stays in the details dialog.
export function entrySize(entry: RemoteEntry): string {
  return entryKind(entry) === "file" ? formatBytes(entry.size) : "—";
}

export function entryTypeLabelKey(entry: RemoteEntry) {
  const fileKind = entry.type === "file" ? fileKindOf(entry.name) : null;
  return fileKind === null ? entryTypeLabelKeys[entry.type] : fileKindLabelKeys[fileKind];
}

// A file is drawn as its kind (a picture, a recording…), so a folder of photos reads at a glance.
export function EntryIcon({ entry, className, folderColour = "text-accent" }: { entry: RemoteEntry; className: string; folderColour?: string }) {
  const kind = entryKind(entry);
  if (kind === "directory") return <Icon name="folder" className={`${className} ${folderColour}`} />;
  const fileKind = kind === "file" ? fileKindOf(entry.name) : null;
  const icon = fileKind === null ? "file" : fileKindIcons[fileKind];
  const colour = (fileKind === null ? undefined : fileKindIconColours[fileKind]) ?? "text-ink-muted";
  return <Icon name={icon} className={`${className} ${colour}`} />;
}

// A symlink is named with where it points, as `ls -l` does.
export function EntryName({ entry }: { entry: RemoteEntry }) {
  const t = useTranslate();
  if (entry.type !== "symlink") return <>{entry.name}</>;
  const target = entry.targetType === undefined ? t("sftp.brokenLink", { target: entry.linkTarget ?? "" }) : entry.linkTarget ?? "";
  return <>{entry.name}<span className="font-normal text-ink-muted">{` → ${target}`}</span></>;
}

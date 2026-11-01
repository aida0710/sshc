import type { MessageKey } from "../i18n/messages";
import type { IconName } from "../ui/icons";
import { playableMediaKind } from "./playableMedia";

// The kinds of file a file list tells apart by name, so pictures and recordings are found without opening
// them. Audio and video follow the list the preview plays (playableMedia.ts).
export type FileKind = "image" | "video" | "audio" | "archive" | "text";

const imageExtensions = ["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "avif", "heic", "heif", "tif", "tiff", "ico"];
const archiveExtensions = ["zip", "tar", "gz", "tgz", "bz2", "xz", "zst", "7z", "rar"];
const textExtensions = [
  "txt", "md", "log", "csv", "json", "yaml", "yml", "toml", "ini", "conf", "cfg", "env", "xml", "html", "css",
  "sh", "bash", "zsh", "py", "go", "rs", "js", "ts", "tsx", "jsx", "c", "h", "cpp", "java", "rb", "php", "sql",
];

export function fileKindOf(name: string): FileKind | null {
  const media = playableMediaKind(name);
  if (media !== null) return media;
  const extension = /\.([^./]+)$/u.exec(name)?.[1]?.toLowerCase() ?? "";
  if (imageExtensions.includes(extension)) return "image";
  if (archiveExtensions.includes(extension)) return "archive";
  if (textExtensions.includes(extension)) return "text";
  return null;
}

export const fileKindIcons: Record<FileKind, IconName> = {
  image: "image",
  video: "film",
  audio: "music",
  archive: "archive",
  text: "fileText",
};

// Pictures and recordings are tinted; the other kinds keep the list's own colour.
export const fileKindIconColours: Partial<Record<FileKind, string>> = {
  image: "text-kind-image",
  video: "text-kind-video",
};

export const fileKindLabelKeys: Record<FileKind, MessageKey> = {
  image: "sftp.type.image",
  video: "sftp.type.video",
  audio: "sftp.type.audio",
  archive: "sftp.type.archive",
  text: "sftp.type.text",
};

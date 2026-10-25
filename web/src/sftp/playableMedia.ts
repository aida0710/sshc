// The audio and video the preview plays, by extension: the engine serves the
// same list (internal/sftp/media_types.go) and refuses a ticket for anything
// else. The name decides before anything is read, so a video is never fetched
// whole just to learn that it is not an image or text.
const audioExtensions = ["mp3", "wav", "m4a", "aac", "ogg", "oga", "opus", "flac"];
const videoExtensions = ["mp4", "m4v", "webm", "mov", "mkv", "ogv"];

export type PlayableMediaKind = "audio" | "video";

export function playableMediaKind(path: string): PlayableMediaKind | null {
  const extension = /\.([^./]+)$/u.exec(path)?.[1]?.toLowerCase() ?? "";
  if (audioExtensions.includes(extension)) return "audio";
  if (videoExtensions.includes(extension)) return "video";
  return null;
}

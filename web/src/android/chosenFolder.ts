import { newIdentifier } from "../ui/randomIdentifier";

// The Android WebView's file input cannot pick a folder (webkitdirectory picks
// nothing), so the Android app opens its own folder picker, lists the folder,
// and serves each file at a same-origin URL the app answers itself.

export type ChosenFolderFile = { file: File; relativePath: string };
export type ChosenFolder = {
  // Every folder in the choice, including empty ones, so they are created too.
  directories: string[];
  files: { relativePath: string; size: number; load: () => Promise<ChosenFolderFile> }[];
  // Stops the app from serving the chosen files once they are read.
  release: () => void;
};

type FolderEvent = CustomEvent<{ requestId?: unknown; status?: unknown }>;
type Listing = { directories: string[]; files: { path: string; url: string; size: number; lastModified: number }[] };

export function canChooseAndroidFolder(target: Window = window): boolean {
  return target.sshcAndroid !== undefined;
}

function isListing(value: unknown): value is Listing {
  if (typeof value !== "object" || value === null) return false;
  const { directories, files } = value as Partial<Listing>;
  return Array.isArray(directories) && directories.every((path) => typeof path === "string")
    && Array.isArray(files) && files.every((entry) => typeof entry === "object" && entry !== null
      && typeof entry.path === "string" && typeof entry.url === "string"
      && typeof entry.size === "number" && typeof entry.lastModified === "number");
}

// Opens the Android folder picker. Resolves to null when the user backs out.
// Throws android_folder_too_many when the folder holds more files than the
// app lists, and android_folder_failed when it could not be read.
export async function chooseAndroidFolder(target: Window = window): Promise<ChosenFolder | null> {
  const bridge = target.sshcAndroid;
  if (bridge === undefined) return null;
  const id = newIdentifier();
  const status = await new Promise<string>((resolve, reject) => {
    const listener = (event: Event) => {
      const detail = (event as FolderEvent).detail;
      if (detail?.requestId !== id || typeof detail.status !== "string") return;
      target.removeEventListener("sshc-android-folder", listener);
      resolve(detail.status);
    };
    target.addEventListener("sshc-android-folder", listener);
    const failure = bridge.chooseFolder(id);
    if (failure !== "") {
      target.removeEventListener("sshc-android-folder", listener);
      reject(new Error(failure));
    }
  });
  if (status === "cancelled") return null;
  if (status === "too_many") throw new Error("android_folder_too_many");
  let listing: unknown = null;
  try {
    listing = status === "ready" ? JSON.parse(bridge.takeChosenFolder(id)) : null;
  } catch {
    listing = null;
  }
  if (!isListing(listing)) throw new Error("android_folder_failed");
  return {
    directories: listing.directories,
    files: listing.files.map((entry) => ({
      relativePath: entry.path,
      size: entry.size,
      load: async () => {
        const response = await target.fetch(entry.url, { cache: "no-store" });
        if (!response.ok) throw new Error("android_folder_failed");
        const name = entry.path.split("/").at(-1) ?? entry.path;
        return { relativePath: entry.path, file: new File([await response.blob()], name, { lastModified: entry.lastModified }) };
      },
    })),
    release: () => bridge.releaseChosenFolder(),
  };
}

// Reading every file at once would hold the whole folder in memory at the
// same moment on a phone; a few at a time keeps the copy short and bounded.
const parallelFileReads = 4;

export async function loadChosenFolderFiles(files: ChosenFolder["files"]): Promise<ChosenFolderFile[]> {
  const loaded: ChosenFolderFile[] = new Array(files.length);
  let next = 0;
  const reader = async () => {
    while (next < files.length) {
      const index = next;
      next += 1;
      const entry = files[index];
      if (entry !== undefined) loaded[index] = await entry.load();
    }
  };
  await Promise.all(Array.from({ length: Math.min(parallelFileReads, files.length) }, reader));
  return loaded;
}

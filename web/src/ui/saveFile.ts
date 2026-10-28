import { saveWithAndroid } from "../android/native";

// A save the browser carries on by itself after saveFile returns: it reads
// the blob later and never says when it has finished. The caller keeps what
// backs the blob and calls release once nothing will read it again.
export type BrowserSave = { release(): void };

// saveFile saves blob as name: in the Android app through its file picker,
// otherwise as the browser's download. It resolves to null when the bytes
// are in the saved file already (Android copies them before returning).
export async function saveFile(blob: Blob, name: string): Promise<BrowserSave | null> {
  if (await saveWithAndroid(blob, name)) return null;
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = name;
  anchor.click();
  // WebView and Safari may resolve the object URL after click() returns.
  return { release: () => URL.revokeObjectURL(url) };
}

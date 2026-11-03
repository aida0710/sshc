// A data: URL keeps a picture inside the policy the app already ships: img-src allows data:, and nothing here needs a
// Blob URL.
export function blobDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error ?? new Error("preview_unreadable"));
    reader.readAsDataURL(blob);
  });
}

type AndroidBridge = {
  chooseSave(requestId: string, suggestedName: string, mimeType: string): string;
  writeSaveChunk(requestId: string, encoded: string): string;
  finishSave(requestId: string): string;
  cancelSave(requestId: string): void;
  notifyTransfer(status: "completed" | "failed"): void;
  setAppearance(appearance: "light" | "dark"): void;
};

declare global {
  interface Window {
    sshcAndroid?: AndroidBridge;
  }
}

type SaveEvent = CustomEvent<{ requestId?: unknown; status?: unknown }>;

const saveChunkSize = 256 * 1024;

function requestId(): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid !== undefined) return uuid.replaceAll("-", "");
  return `${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}_save`;
}

function base64(bytes: Uint8Array): string {
  let binary = "";
  for (let offset = 0; offset < bytes.byteLength; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + 0x8000, bytes.byteLength)));
  }
  return globalThis.btoa(binary);
}

/** Android版ではStorage Access Frameworkへ保存する。Web版ならfalseを返して通常downloadへ委ねる。 */
export async function saveWithAndroid(blob: Blob, suggestedName: string): Promise<boolean> {
  const bridge = window.sshcAndroid;
  if (bridge === undefined) return false;
  const id = requestId();
  const status = await new Promise<string>((resolve, reject) => {
    const listener = (event: Event) => {
      const detail = (event as SaveEvent).detail;
      if (detail?.requestId !== id || typeof detail.status !== "string") return;
      window.removeEventListener("sshc-android-save", listener);
      resolve(detail.status);
    };
    window.addEventListener("sshc-android-save", listener);
    const failure = bridge.chooseSave(id, suggestedName, blob.type || "application/octet-stream");
    if (failure !== "") {
      window.removeEventListener("sshc-android-save", listener);
      reject(new Error(failure));
    }
  });
  if (status === "cancelled") throw new Error("android_save_cancelled");
  if (status !== "ready") throw new Error("android_save_failed");

  try {
    for (let offset = 0; offset < blob.size; offset += saveChunkSize) {
      const bytes = new Uint8Array(await blob.slice(offset, offset + saveChunkSize).arrayBuffer());
      const failure = bridge.writeSaveChunk(id, base64(bytes));
      if (failure !== "") throw new Error(failure);
    }
    const failure = bridge.finishSave(id);
    if (failure !== "") throw new Error(failure);
  } catch (error) {
    bridge.cancelSave(id);
    throw error;
  }
  return true;
}

/** Android通知はファイル名を含めず、結果だけを端末へ知らせる。 */
export function notifyAndroidTransfer(status: "completed" | "failed"): void {
  try {
    window.sshcAndroid?.notifyTransfer(status);
  } catch {
    // WebView bridgeがrenderer再起動と同時に消えてもWeb側の転送結果は維持する。
  }
}

export function setAndroidAppearance(appearance: "light" | "dark"): void {
  try {
    window.sshcAndroid?.setAppearance(appearance);
  } catch {
    // Android以外、またはrenderer再起動中はWebのthemeだけを維持する。
  }
}

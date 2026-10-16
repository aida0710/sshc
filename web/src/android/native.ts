import { newIdentifier } from "../ui/randomIdentifier";

type AndroidBridge = {
  chooseSave(requestId: string, suggestedName: string, mimeType: string): string;
  writeSaveChunk(requestId: string, encoded: string): string;
  finishSave(requestId: string): string;
  cancelSave(requestId: string): void;
  notifyTransfer(status: "completed" | "failed"): void;
  setAppearance(appearance: "light" | "dark"): void;
  notificationPermission(): string;
  requestNotificationPermission(): void;
  showNotification(tag: string, title: string, body: string): string;
};

type AndroidWindow = Window & { sshcAndroid?: AndroidBridge };
type AndroidNotificationPermission = "granted" | "denied" | "default";
type NotificationClickEvent = CustomEvent<{ tag?: unknown }>;
type NotificationPermissionEvent = CustomEvent<{ status?: unknown }>;

// 通知を押してアプリへ戻ったときに呼ぶ処理。tag ごとに最後の通知の分だけ持つ。
const notificationClicks = new Map<string, () => void>();
const listeningForClicks = new WeakSet<Window>();

declare global {
  interface Window {
    sshcAndroid?: AndroidBridge;
  }
}

type SaveEvent = CustomEvent<{ requestId?: unknown; status?: unknown }>;

const saveChunkSize = 256 * 1024;

function base64(bytes: Uint8Array): string {
  let binary = "";
  for (let offset = 0; offset < bytes.byteLength; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + 0x8000, bytes.byteLength)));
  }
  return globalThis.btoa(binary);
}

/** Androidの WebView は webkitdirectory の input でもフォルダを選ばせず、選んだファイルも渡さない。 */
export function canChooseFolders(): boolean {
  return window.sshcAndroid === undefined;
}

/** Android版は転送の結果を transferLedger が notifyAndroidTransfer で知らせる。 */
export function notifiesTransfersNatively(): boolean {
  return window.sshcAndroid !== undefined;
}

function androidPermission(value: string): AndroidNotificationPermission {
  return value === "granted" || value === "denied" ? value : "default";
}

/** Android版の通知の許可の状態。Android版でなければ null。 */
export function androidNotificationPermission(target: Window = window): AndroidNotificationPermission | null {
  const bridge = (target as AndroidWindow).sshcAndroid;
  return bridge === undefined ? null : androidPermission(bridge.notificationPermission());
}

/** Android の通知の許可を求め、答えを待つ。 */
export function requestAndroidNotificationPermission(target: Window = window): Promise<AndroidNotificationPermission> {
  const bridge = (target as AndroidWindow).sshcAndroid;
  if (bridge === undefined) return Promise.resolve("default");
  return new Promise((resolve) => {
    const listener = (event: Event) => {
      const status = (event as NotificationPermissionEvent).detail?.status;
      if (typeof status !== "string") return;
      target.removeEventListener("sshc-android-notification-permission", listener);
      resolve(androidPermission(status));
    };
    target.addEventListener("sshc-android-notification-permission", listener);
    bridge.requestNotificationPermission();
  });
}

/** Android の通知を出す。出せたら true。押されたら onClick を呼ぶ。 */
export function showAndroidNotification(
  { title, body, tag, onClick }: { title: string; body: string; tag: string; onClick?: (() => void) | undefined },
  target: Window = window,
): boolean {
  const bridge = (target as AndroidWindow).sshcAndroid;
  if (bridge === undefined) return false;
  try {
    if (bridge.showNotification(tag, title, body) !== "") return false;
  } catch {
    // renderer の再起動と同時に bridge が消えても、通知が出なかったことにする。
    return false;
  }
  if (onClick === undefined) notificationClicks.delete(tag);
  else notificationClicks.set(tag, onClick);
  if (!listeningForClicks.has(target)) {
    listeningForClicks.add(target);
    target.addEventListener("sshc-android-notification-click", (event: Event) => {
      const clicked = (event as NotificationClickEvent).detail?.tag;
      if (typeof clicked !== "string") return;
      const handler = notificationClicks.get(clicked);
      notificationClicks.delete(clicked);
      handler?.();
    });
  }
  return true;
}

/** Android版ではStorage Access Frameworkへ保存する。Web版ならfalseを返して通常downloadへ委ねる。 */
export async function saveWithAndroid(blob: Blob, suggestedName: string): Promise<boolean> {
  const bridge = window.sshcAndroid;
  if (bridge === undefined) return false;
  const id = newIdentifier();
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

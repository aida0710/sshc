import type { ColorScheme } from "../theme/theme";
import type { HapticFeedback } from "./haptics";
import { newIdentifier } from "../ui/randomIdentifier";

export type AndroidBridge = {
  chooseSave(requestId: string, suggestedName: string, mimeType: string): string;
  writeSaveChunk(requestId: string, encoded: string): string;
  finishSave(requestId: string): string;
  cancelSave(requestId: string): void;
  notifyTransfer(status: "completed" | "failed"): void;
  setAppearance(appearance: ColorScheme, barColour: string): void;
  notificationPermission(): string;
  requestNotificationPermission(): void;
  showNotification(tag: string, title: string, body: string): string;
  showBlockedProgramNotification(tag: string, title: string, body: string): string;
  cancelNotification(tag: string): void;
  takeOpenedNotification(): string;
  chooseFolder(requestId: string): string;
  takeChosenFolder(requestId: string): string;
  releaseChosenFolder(): void;
  performHapticFeedback(feedback: HapticFeedback): void;
  firstScreenShown(): void;
};

type AndroidWindow = Window & { sshcAndroid?: AndroidBridge };
type AndroidNotificationPermission = "granted" | "denied" | "default";
type NotificationPermissionEvent = CustomEvent<{ status?: unknown }>;

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

/**
 * このページが Android のアプリの WebView の中で動いているか。アプリには、ブラウザのタブも
 * コマンドラインも、ファイルを開く道具も無い。
 */
export function runsInAndroidApp(target: Window = window): boolean {
  return (target as AndroidWindow).sshcAndroid !== undefined;
}

/** Android版は転送の結果を transferLedger が notifyAndroidTransfer で知らせる。 */
export function notifiesTransfersNatively(): boolean {
  return runsInAndroidApp();
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

/**
 * Android の通知を出す。出せたら true。押されたときの処理は持たない。通知を押して
 * 開いたときには画面を読み込み直していることがあるので、押された通知は
 * subscribeAndroidNotificationOpened が tag で知らせる。
 */
export function showAndroidNotification(
  { title, body, tag }: { title: string; body: string; tag: string },
  target: Window = window,
): boolean {
  const bridge = (target as AndroidWindow).sshcAndroid;
  if (bridge === undefined) return false;
  try {
    return bridge.showNotification(tag, title, body) === "";
  } catch {
    // renderer の再起動と同時に bridge が消えても、通知が出なかったことにする。
    return false;
  }
}

/** プログラムが利用者を待っていることの通知。音が鳴るチャンネルで出し、同じ待ちは1回だけ出す。 */
export function showAndroidBlockedProgramNotification(
  { title, body, tag }: { title: string; body: string; tag: string },
  target: Window = window,
): boolean {
  const bridge = (target as AndroidWindow).sshcAndroid;
  if (bridge === undefined) return false;
  try {
    return bridge.showBlockedProgramNotification(tag, title, body) === "";
  } catch {
    // renderer の再起動と同時に bridge が消えても、通知が出なかったことにする。
    return false;
  }
}

/** Android の通知を取り下げる。出していなければ何もしない。 */
export function cancelAndroidNotification(tag: string, target: Window = window): void {
  try {
    (target as AndroidWindow).sshcAndroid?.cancelNotification(tag);
  } catch {
    // renderer の再起動と同時に bridge が消えた。通知は押されるか払われるまで残る。
  }
}

/**
 * 押された通知の tag を知らせる。登録したとき（閉じていたアプリや読み込み直した画面を
 * 通知から開いた場合）と、開いている画面で通知が押されたときに呼ぶ。Android は押された tag を
 * 1 度しか渡さないので、登録するのは 1 か所だけにする。Android版でなければ何もしない。
 */
export function subscribeAndroidNotificationOpened(opened: (tag: string) => void, target: Window = window): () => void {
  const bridge = (target as AndroidWindow).sshcAndroid;
  if (bridge === undefined) return () => {};
  const takeOpened = () => {
    let tag = "";
    try {
      tag = bridge.takeOpenedNotification();
    } catch {
      // renderer の再起動と同時に bridge が消えた。次に読み込んだ画面が受け取る。
      return;
    }
    if (tag !== "") opened(tag);
  };
  target.addEventListener("sshc-android-notification-opened", takeOpened);
  takeOpened();
  return () => target.removeEventListener("sshc-android-notification-opened", takeOpened);
}

// AndroidSaveCancelled is the person leaving the save destination without choosing one. It is their decision, not a
// failure to report.
export class AndroidSaveCancelled extends Error {
  constructor() {
    super("android_save_cancelled");
  }
}

// The save before this one, finished or not. The app opens one save destination at a time and fails a request made while
// one is open (MainActivity.openSaveDestination), and transfers run side by side, so a save waits for the one before it.
let previousSave: Promise<unknown> = Promise.resolve();

/** Android版ではStorage Access Frameworkへ保存する。Web版ならfalseを返して通常downloadへ委ねる。 */
export function saveWithAndroid(blob: Blob, suggestedName: string): Promise<boolean> {
  const bridge = window.sshcAndroid;
  if (bridge === undefined) return Promise.resolve(false);
  const save = previousSave.then(() => saveThroughBridge(bridge, blob, suggestedName));
  // A cancelled or failed save does not stop the next: each file has its own destination to choose.
  previousSave = save.catch(() => undefined);
  return save;
}

async function saveThroughBridge(bridge: AndroidBridge, blob: Blob, suggestedName: string): Promise<boolean> {
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
  if (status === "cancelled") throw new AndroidSaveCancelled();
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

// longHexColour writes #rgb as #rrggbb, the only form the Android bridge takes. The theme files use six digits, but
// the CSS minifier shortens the light theme's white to three in the built app.
function longHexColour(colour: string): string {
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(colour);
  return short === null ? colour : `#${short[1]}${short[1]}${short[2]}${short[2]}${short[3]}${short[3]}`;
}

// setAndroidAppearance paints Android's status and navigation bars like the top bar of the theme on screen:
// the theme's --ui-toolbar, with light or dark icons by its scheme.
export function setAndroidAppearance(scheme: ColorScheme): void {
  const barColour = longHexColour(getComputedStyle(document.documentElement).getPropertyValue("--ui-toolbar").trim());
  try {
    window.sshcAndroid?.setAppearance(scheme, barColour);
  } catch {
    // Android以外、またはrenderer再起動中はWebのthemeだけを維持する。
  }
}

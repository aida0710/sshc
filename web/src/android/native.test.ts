import { afterEach, describe, expect, it, vi } from "vitest";
import {
  androidNotificationPermission,
  canChooseFolders,
  notifiesTransfersNatively,
  notifyAndroidTransfer,
  requestAndroidNotificationPermission,
  saveWithAndroid,
  setAndroidAppearance,
  showAndroidNotification,
} from "./native";

type AndroidBridge = NonNullable<Window["sshcAndroid"]>;

function fakeBridge(overrides: Partial<AndroidBridge> = {}): AndroidBridge {
  return {
    chooseSave: vi.fn(() => ""),
    writeSaveChunk: vi.fn(() => ""),
    finishSave: vi.fn(() => ""),
    cancelSave: vi.fn(),
    notifyTransfer: vi.fn(),
    setAppearance: vi.fn(),
    notificationPermission: vi.fn(() => "granted"),
    requestNotificationPermission: vi.fn(),
    showNotification: vi.fn(() => ""),
    ...overrides,
  };
}

afterEach(() => {
  delete window.sshcAndroid;
});

describe("Android native bridge", () => {
  it("Android以外では通常のdownloadへ委ねる", async () => {
    await expect(saveWithAndroid(new Blob(["sshc"]), "file.txt")).resolves.toBe(false);
  });

  it("Android版ではフォルダを選ばせない", () => {
    expect(canChooseFolders()).toBe(true);
    window.sshcAndroid = fakeBridge();
    expect(canChooseFolders()).toBe(false);
  });

  it("選択した保存先へ分割した内容を書き込む", async () => {
    const chunks: string[] = [];
    const finishSave = vi.fn(() => "");
    window.sshcAndroid = fakeBridge({
      chooseSave(requestId) {
        window.dispatchEvent(new CustomEvent("sshc-android-save", {
          detail: { requestId, status: "ready" },
        }));
        return "";
      },
      writeSaveChunk(_requestId, encoded) {
        chunks.push(encoded);
        return "";
      },
      finishSave,
    });

    await expect(saveWithAndroid(new Blob(["sshc"], { type: "text/plain" }), "file.txt"))
      .resolves.toBe(true);
    expect(chunks.map((value) => atob(value)).join("")).toBe("sshc");
    expect(finishSave).toHaveBeenCalledOnce();
  });

  it("許可された転送結果だけをnativeへ通知する", () => {
    const notifyTransfer = vi.fn();
    window.sshcAndroid = fakeBridge({ notifyTransfer });
    notifyAndroidTransfer("completed");
    expect(notifyTransfer).toHaveBeenCalledWith("completed");
  });

  it("Android版では転送の結果をnative側だけが知らせる", () => {
    expect(notifiesTransfersNatively()).toBe(false);
    window.sshcAndroid = fakeBridge();
    expect(notifiesTransfersNatively()).toBe(true);
  });

  it("Webの配色をAndroidのsystem barへ同期する", () => {
    const setAppearance = vi.fn();
    window.sshcAndroid = fakeBridge({ setAppearance });
    setAndroidAppearance("dark");
    expect(setAppearance).toHaveBeenCalledWith("dark");
  });
});

describe("Android notifications", () => {
  it("Android以外では許可の状態を持たない", () => {
    expect(androidNotificationPermission()).toBeNull();
  });

  it("知らない許可の状態はまだ聞いていないものとして扱う", () => {
    window.sshcAndroid = fakeBridge({ notificationPermission: () => "unexpected" });
    expect(androidNotificationPermission()).toBe("default");
  });

  it("許可を求めてAndroidの答えを返す", async () => {
    window.sshcAndroid = fakeBridge({
      requestNotificationPermission() {
        window.dispatchEvent(new CustomEvent("sshc-android-notification-permission", { detail: { status: "granted" } }));
      },
    });
    await expect(requestAndroidNotificationPermission()).resolves.toBe("granted");
  });

  it("通知を渡し、押されたらその通知の処理だけを呼ぶ", () => {
    const showNotification = vi.fn(() => "");
    window.sshcAndroid = fakeBridge({ showNotification });
    const opened = vi.fn();
    const other = vi.fn();
    expect(showAndroidNotification({ title: "build", body: "done", tag: "sshc-terminal-1", onClick: opened })).toBe(true);
    expect(showAndroidNotification({ title: "test", body: "done", tag: "sshc-terminal-2", onClick: other })).toBe(true);
    expect(showNotification).toHaveBeenCalledWith("sshc-terminal-1", "build", "done");

    window.dispatchEvent(new CustomEvent("sshc-android-notification-click", { detail: { tag: "sshc-terminal-1" } }));
    window.dispatchEvent(new CustomEvent("sshc-android-notification-click", { detail: { tag: "sshc-terminal-1" } }));
    expect(opened).toHaveBeenCalledOnce();
    expect(other).not.toHaveBeenCalled();
  });

  it("Androidが通知を断ったら出せなかったと返す", () => {
    window.sshcAndroid = fakeBridge({ showNotification: () => "notifications_blocked" });
    expect(showAndroidNotification({ title: "build", body: "done", tag: "sshc-terminal-1" })).toBe(false);
  });
});

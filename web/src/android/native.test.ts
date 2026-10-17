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
  subscribeAndroidNotificationOpened,
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
    takeOpenedNotification: vi.fn(() => ""),
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

  it("通知の題名・本文・tagをAndroidへ渡す", () => {
    const showNotification = vi.fn(() => "");
    window.sshcAndroid = fakeBridge({ showNotification });
    expect(showAndroidNotification({ title: "build", body: "done", tag: "sshc-terminal-1" })).toBe(true);
    expect(showNotification).toHaveBeenCalledWith("sshc-terminal-1", "build", "done");
  });

  it("通知から開いた画面は、登録したときに押された通知を受け取る", () => {
    let pending = "sshc-terminal-1";
    window.sshcAndroid = fakeBridge({ takeOpenedNotification: () => { const tag = pending; pending = ""; return tag; } });
    const opened = vi.fn();
    const stop = subscribeAndroidNotificationOpened(opened);
    expect(opened).toHaveBeenCalledExactlyOnceWith("sshc-terminal-1");
    stop();
  });

  it("開いている画面で通知が押されたら、知らせを受けて1度だけ受け取る", () => {
    let pending = "";
    window.sshcAndroid = fakeBridge({ takeOpenedNotification: () => { const tag = pending; pending = ""; return tag; } });
    const opened = vi.fn();
    const stop = subscribeAndroidNotificationOpened(opened);
    expect(opened).not.toHaveBeenCalled();
    pending = "sshc-command-2";
    window.dispatchEvent(new CustomEvent("sshc-android-notification-opened"));
    window.dispatchEvent(new CustomEvent("sshc-android-notification-opened"));
    expect(opened).toHaveBeenCalledExactlyOnceWith("sshc-command-2");
    stop();
    pending = "sshc-terminal-3";
    window.dispatchEvent(new CustomEvent("sshc-android-notification-opened"));
    expect(opened).toHaveBeenCalledOnce();
  });

  it("Androidが通知を断ったら出せなかったと返す", () => {
    window.sshcAndroid = fakeBridge({ showNotification: () => "notifications_blocked" });
    expect(showAndroidNotification({ title: "build", body: "done", tag: "sshc-terminal-1" })).toBe(false);
  });
});

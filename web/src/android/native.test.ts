import { afterEach, describe, expect, it, vi } from "vitest";
import { notifyAndroidTransfer, saveWithAndroid, setAndroidAppearance } from "./native";

afterEach(() => {
  delete window.sshcAndroid;
});

describe("Android native bridge", () => {
  it("Android以外では通常のdownloadへ委ねる", async () => {
    await expect(saveWithAndroid(new Blob(["sshc"]), "file.txt")).resolves.toBe(false);
  });

  it("選択した保存先へ分割した内容を書き込む", async () => {
    const chunks: string[] = [];
    window.sshcAndroid = {
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
      finishSave: vi.fn(() => ""),
      cancelSave: vi.fn(),
      notifyTransfer: vi.fn(),
      setAppearance: vi.fn(),
    };

    await expect(saveWithAndroid(new Blob(["sshc"], { type: "text/plain" }), "file.txt"))
      .resolves.toBe(true);
    expect(chunks.map((value) => atob(value)).join("")).toBe("sshc");
    expect(window.sshcAndroid.finishSave).toHaveBeenCalledOnce();
  });

  it("許可された転送結果だけをnativeへ通知する", () => {
    const notifyTransfer = vi.fn();
    window.sshcAndroid = {
      chooseSave: vi.fn(() => ""),
      writeSaveChunk: vi.fn(() => ""),
      finishSave: vi.fn(() => ""),
      cancelSave: vi.fn(),
      notifyTransfer,
      setAppearance: vi.fn(),
    };
    notifyAndroidTransfer("completed");
    expect(notifyTransfer).toHaveBeenCalledWith("completed");
  });

  it("Webの配色をAndroidのsystem barへ同期する", () => {
    const setAppearance = vi.fn();
    window.sshcAndroid = {
      chooseSave: vi.fn(() => ""),
      writeSaveChunk: vi.fn(() => ""),
      finishSave: vi.fn(() => ""),
      cancelSave: vi.fn(),
      notifyTransfer: vi.fn(),
      setAppearance,
    };
    setAndroidAppearance("dark");
    expect(setAppearance).toHaveBeenCalledWith("dark");
  });
});

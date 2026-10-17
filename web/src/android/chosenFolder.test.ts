import { afterEach, describe, expect, it, vi } from "vitest";
import { canChooseAndroidFolder, chooseAndroidFolder, loadChosenFolderFiles } from "./chosenFolder";
import { fakeAndroidBridge } from "./fakeAndroidBridge";

function folderBridge(status: string, listing: string) {
  return fakeAndroidBridge({
    chooseFolder(requestId) {
      queueMicrotask(() => window.dispatchEvent(new CustomEvent("sshc-android-folder", { detail: { requestId, status } })));
      return "";
    },
    takeChosenFolder: vi.fn(() => listing),
  });
}

const listing = JSON.stringify({
  directories: ["photos", "photos/empty", "photos/2026"],
  files: [
    { path: "photos/2026/a.txt", url: "/__sshc-android-folder/aaa", size: 5, lastModified: 1_700_000_000_000 },
    { path: "photos/b.txt", url: "/__sshc-android-folder/bbb", size: 3, lastModified: 0 },
  ],
});

afterEach(() => {
  delete window.sshcAndroid;
  vi.unstubAllGlobals();
});

describe("chooseAndroidFolder", () => {
  it("Android版でだけ使える", () => {
    expect(canChooseAndroidFolder()).toBe(false);
    window.sshcAndroid = folderBridge("cancelled", "");
    expect(canChooseAndroidFolder()).toBe(true);
  });

  it("選んだフォルダの一覧を返し、ファイルは読むときに中身を受け取る", async () => {
    const bridge = folderBridge("ready", listing);
    window.sshcAndroid = bridge;
    // jsdom's File only reads jsdom's own Blob, not the one Node's Response returns.
    const fetchFile = vi.fn(async (url: string) => ({ ok: true, blob: async () => new Blob([url.endsWith("aaa") ? "hello" : "bye"]) }));
    vi.stubGlobal("fetch", fetchFile);
    const chosen = await chooseAndroidFolder();
    expect(chosen?.directories).toEqual(["photos", "photos/empty", "photos/2026"]);
    expect(chosen?.files.map((file) => [file.relativePath, file.size])).toEqual([["photos/2026/a.txt", 5], ["photos/b.txt", 3]]);
    expect(fetchFile).not.toHaveBeenCalled();

    const files = await loadChosenFolderFiles(chosen!.files);
    expect(files.map(({ relativePath, file }) => [relativePath, file.name, file.lastModified])).toEqual([
      ["photos/2026/a.txt", "a.txt", 1_700_000_000_000],
      ["photos/b.txt", "b.txt", 0],
    ]);
    expect(await files[0]?.file.text()).toBe("hello");
    chosen!.release();
    expect(bridge.releaseChosenFolder).toHaveBeenCalled();
  });

  it("選ぶのをやめたらnullを返す", async () => {
    window.sshcAndroid = folderBridge("cancelled", "");
    await expect(chooseAndroidFolder()).resolves.toBeNull();
  });

  it("ファイルが多すぎる、読めないときは理由のcodeで失敗する", async () => {
    window.sshcAndroid = folderBridge("too_many", "");
    await expect(chooseAndroidFolder()).rejects.toThrow("android_folder_too_many");
    window.sshcAndroid = folderBridge("ready", "{\"files\":3}");
    await expect(chooseAndroidFolder()).rejects.toThrow("android_folder_failed");
  });
});

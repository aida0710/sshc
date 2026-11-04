import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "./fakeAndroidBridge";
import { shownFilePath } from "./shownFilePath";

describe("shownFilePath", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("names a file under ~/.ssh that way in the Android app", () => {
    vi.stubGlobal("sshcAndroid", fakeAndroidBridge());
    expect(shownFilePath("/data/data/com.example/files/.ssh/conf.d/10-home.conf")).toBe("~/.ssh/conf.d/10-home.conf");
  });

  it("leaves a relative path in the Android app as it is", () => {
    vi.stubGlobal("sshcAndroid", fakeAndroidBridge());
    expect(shownFilePath("config")).toBe("config");
  });

  it("leaves a browser's paths as they are", () => {
    expect(shownFilePath("/home/tester/.ssh/config")).toBe("/home/tester/.ssh/config");
  });
});

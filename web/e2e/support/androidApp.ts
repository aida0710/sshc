import type { Page } from "@playwright/test";

// The Android app is this page in a WebView with its bridge on window, on an engine that says it runs on Android. Those
// two are all the page goes by, so they are all this file stands in for. Every bridge call answers as an idle app would.
export async function behaveAsAndroidApp(page: Page) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.addInitScript(() => {
    Object.defineProperty(window, "sshcAndroid", { value: new Proxy({}, { get: () => () => "" }) });
  });
  await page.route("**/api/v1/about", (route) => route.fulfill({ json: {
    version: "v1.0.0", revision: "", goVersion: "go1.26.10", os: "android", arch: "arm64", manager: "unknown", executable: "/system/bin/app_process64",
  } }));
}

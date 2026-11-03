import type { Page } from "@playwright/test";
import { expect, openApplication, sessionStatus, test, type Installation } from "./support/environment";

// The Android app is this page in a WebView with its bridge on window, on an engine that says it runs on Android. Those
// two are all the page goes by, so they are all this file stands in for. Every bridge call answers as an idle app would.
async function behaveAsAndroidApp(page: Page) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.addInitScript(() => {
    Object.defineProperty(window, "sshcAndroid", { value: new Proxy({}, { get: () => () => "" }) });
  });
  await page.route("**/api/v1/about", (route) => route.fulfill({ json: {
    version: "v1.0.0", revision: "", goVersion: "go1.26.10", os: "android", arch: "arm64", manager: "unknown", executable: "/system/bin/app_process64",
  } }));
}

// The engine answers the settings with the machine it runs on; this makes that machine the phone, named as the engine
// names a machine whose system gives no host name.
async function makeThisMachineThePhone(page: Page) {
  const read = await page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/settings" && response.request().method() === "GET" && response.ok());
  const settings = await read.json() as { machines: { settings: { name?: string }; facts: { os?: string } }[] };
  for (const machine of settings.machines) {
    machine.settings.name = "android";
    machine.facts.os = "android";
  }
  await page.route("**/api/v1/settings", (route) => route.request().method() === "GET" ? route.fulfill({ json: settings }) : route.fallback());
}

async function openFromMenu(page: Page, name: string) {
  await expect(sessionStatus(page)).toContainText("Local session active");
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: `Open ${name}`, exact: true }).click();
}

async function openAndroidApp(page: Page, installation: Installation) {
  await behaveAsAndroidApp(page);
  await openApplication(page, installation);
}

test("the Android app's lock screen and menu offer no sign-out", async ({ page, installation }) => {
  await behaveAsAndroidApp(page);
  await page.goto(installation.url);
  await expect(page.getByLabel("Master password", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign out of this browser" })).toHaveCount(0);

  await page.getByLabel("Master password", { exact: true }).fill("an end to end master password");
  await page.getByLabel("Confirm master password", { exact: true }).fill("an end to end master password");
  await page.getByRole("button", { name: "Create the vault" }).click();
  await expect(sessionStatus(page)).toContainText("Local session active");
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Menu", exact: true }).click();
  await expect(page.getByRole("link", { name: "Open About sshc", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign out of this browser" })).toHaveCount(0);
});

test("the Android app's settings speak of the device and offer no port", async ({ page, installation }) => {
  await openAndroidApp(page, installation);
  await makeThisMachineThePhone(page);
  await openFromMenu(page, "General");
  await expect(page.getByLabel("Display language").getByRole("option", { name: "Follow the device" })).toBeAttached();
  await expect(page.getByLabel("Display language").getByRole("option", { name: "Follow the browser" })).toHaveCount(0);

  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: "Open Machines", exact: true }).click();
  const machines = page.getByRole("region", { name: "Machines" });
  await expect(machines.getByText("This device")).toBeVisible();
  await expect(machines.getByText("Android", { exact: true })).toBeVisible();
  await expect(machines.getByRole("spinbutton", { name: "Port" })).toHaveCount(0);
  await expect(machines).not.toContainText("54447");

  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: "Open Notifications", exact: true }).click();
  const notifications = page.getByRole("region", { name: "Notifications" });
  await expect(notifications).toContainText("while the app is in the background");
  await expect(notifications).not.toContainText("this tab");
});

test("the Android app's About leaves out how sshc was installed and its executable", async ({ page, installation }) => {
  await openAndroidApp(page, installation);
  await openFromMenu(page, "About sshc");
  const about = page.getByRole("region", { name: "About sshc", exact: true });
  await expect(about.getByText("android / ")).toBeVisible();
  await expect(about.getByText("Installed with")).toHaveCount(0);
  await expect(about.getByText("/system/bin/app_process64")).toHaveCount(0);
});

test("the Android app's key and known hosts pages say nothing of ssh-agent or the app's private folder", async ({ page, installation }) => {
  await openAndroidApp(page, installation);
  await openFromMenu(page, "Keys");
  await expect(page.getByRole("heading", { name: "Keys" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "ssh-agent" })).toHaveCount(0);
  await expect(page.getByText(/SSH_AUTH_SOCK/u)).toHaveCount(0);

  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: "Open Known Hosts", exact: true }).click();
  await expect(page.getByText("~/.ssh/known_hosts", { exact: true }).first()).toBeVisible();
  await expect(page.locator("body")).not.toContainText(installation.home);
});

test("the Android app's history leaves out sshc's own files and where its backups are", async ({ page, installation }) => {
  await page.route("**/api/v1/history", (route) => route.fulfill({ json: { entries: [{
    id: "20260805T120000.000-abcd", operation: "machine.settings.update", status: "completed",
    startedAt: "2026-08-05T12:00:00Z", finishedAt: "2026-08-05T12:00:01Z",
    paths: ["sshc/machines/14c85e4005beb423d6c486828933a604.json"], restorable: [],
  }] } }));
  await openAndroidApp(page, installation);
  await openFromMenu(page, "History");
  await expect(page.getByText("Machine settings change")).toBeVisible();
  await expect(page.getByText(/sshc\/machines/u)).toHaveCount(0);
  await expect(page.getByText("Previous versions of changed files are kept inside the app.")).toBeVisible();
});

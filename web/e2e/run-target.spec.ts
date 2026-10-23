import type { Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

const localVersion = "v0.53.0";
const remoteVersion = "v0.51.0";
const incompatiblePrompt = "Cannot connect to bastion because its sshc version does not work with this computer’s. Update it?";
const sessionsPath = "/api/v1/engines/bastion/terminal/sessions";

// refusal is what the stubbed engine on bastion answers until it is fixed:
// an incompatible version, which the engine information dialog updates, or a
// host that cannot be reached until the test says it can.
async function stubRunTarget(page: Page, { added = true, refusal = "version", compatibleVersion }: { added?: boolean; refusal?: "version" | "unreachable"; compatibleVersion?: string } = {}) {
  let fixed = compatibleVersion !== undefined;
  const actions: unknown[] = [];
  await page.route("**/api/v1/health", async (route) => {
    await route.fulfill({ json: { status: "ok", version: localVersion } });
  });
  await page.route("**/api/v1/engines", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({ alias: "bastion" });
      added = true;
      await route.fulfill({ status: 201, json: { alias: "bastion" } });
    } else {
      await route.fulfill({ json: { engines: added ? [{ alias: "bastion" }] : [] } });
    }
  });
  await page.route("**/api/v1/engines/bastion/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/control")) {
      actions.push(route.request().postDataJSON());
      fixed = true;
      await route.fulfill({ json: { succeeded: true, output: "sshc updated" } });
      return;
    }
    if (!fixed) {
      await route.fulfill(refusal === "version"
        ? { status: 409, contentType: "application/problem+json", json: { code: "remote_engine_incompatible", message: "request rejected", detail: remoteVersion } }
        : { status: 502, contentType: "application/problem+json", json: { code: "remote_engine_unreachable", message: "request rejected" } });
      return;
    }
    if (path.endsWith("/status")) {
      await route.fulfill({ json: { alias: "bastion", version: compatibleVersion ?? localVersion, vault: true, unlocked: true, passwordless: true, sessions: 0 } });
    } else if (path.endsWith("/terminal/sessions")) {
      await route.fulfill({ json: { sessions: [], maxSessions: 50 } });
    } else if (path.endsWith("/terminal/workspaces")) {
      await route.fulfill({ json: { workspaces: [{ id: "default", name: "", hidden: false, builtIn: true }], maxWorkspaces: 50 } });
    } else {
      throw new Error(`Unexpected remote engine request: ${path}`);
    }
  });
  return { actions, fix: () => { fixed = true; } };
}

async function chooseBastion(page: Page) {
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: "bastion", exact: true }).click();
}

// screenshotInJapanese saves the screen, wide and narrow, as a person reading Japanese
// sees it, when SSHC_VISUAL_DIR asks for screenshots.
async function screenshotInJapanese(page: Page, name: string) {
  const directory = process.env.SSHC_VISUAL_DIR;
  if (directory === undefined) return;
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "ja");
  await expect(page.getByRole("dialog").getByRole("button", { name: "再接続", exact: true })).toBeVisible();
  await page.screenshot({ path: `${directory}/${name}.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  // The sidebar slides away at this width; the screenshot waits until it is gone.
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  await page.screenshot({ path: `${directory}/${name}-mobile.png` });
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "en");
}

test("shows an incompatible engine in the terminal area and recovers after updating", async ({ page, installation }) => {
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("dialog", { name: "The engine on bastion runs an sshc version that does not work with this computer’s", exact: true });
  await expect(panel).toBeVisible();
  const information = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(information).toBeHidden();
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-incompatible");

  await panel.getByRole("button", { name: "Update…", exact: true }).click();
  await expect(information.getByText(incompatiblePrompt, { exact: true })).toBeVisible();
  await expect(information.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(information.getByText(`this computer runs ${localVersion}`, { exact: true })).toBeVisible();
  await information.getByRole("button", { name: "Update", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "Update sshc on bastion?", exact: true });
  await confirmation.getByRole("button", { name: "Update", exact: true }).click();
  await expect(information.getByText(incompatiblePrompt, { exact: true })).toBeHidden();
  await expect(information.getByText(localVersion, { exact: true })).toBeVisible();
  expect(remote.actions).toEqual([{ action: "update" }]);
  await information.getByRole("button", { name: "Close", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

test("shows an unreachable host in the terminal area and goes away on Reconnect", async ({ page, installation }) => {
  const remote = await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("dialog", { name: "Could not connect to bastion", exact: true });
  await expect(panel.getByText("The engine terminals run on cannot be reached. Reconnect, or switch back to localhost.", { exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-unreachable");

  // The list is read again every two seconds; right after one reading the
  // next is still far off, so the panel can only go away through Reconnect.
  await page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath);
  remote.fix();
  await Promise.all([
    page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath && response.ok()),
    panel.getByRole("button", { name: "Reconnect", exact: true }).click(),
  ]);
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose where terminals run \(now: bastion\)/ })).toBeVisible();
});

test("switches back to localhost from the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("dialog", { name: "Could not connect to bastion", exact: true });
  await panel.getByRole("button", { name: "Switch back to localhost", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose where terminals run \(now: localhost\)/ })).toBeVisible();
});

test("selects a newly added engine and shows it is incompatible in the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { added: false });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Add an engine…", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "Add the engine on an SSH host", exact: true });
  await picker.getByRole("button", { name: /^bastion/ }).click();
  const panel = page.getByRole("dialog", { name: "The engine on bastion runs an sshc version that does not work with this computer’s", exact: true });
  await panel.getByRole("button", { name: "Update…", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Engine on bastion", exact: true }).getByText(incompatiblePrompt, { exact: true })).toBeVisible();
});

test("uses a compatible engine of another version without a panel", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: remoteVersion });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Actions for engine bastion", exact: true }).click();
  await page.getByRole("button", { name: "Engine information…", exact: true }).click();
  const information = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(information.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(information.getByText(`this computer runs ${localVersion}`, { exact: true })).toBeVisible();
});

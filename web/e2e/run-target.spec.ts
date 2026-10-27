import type { Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

const localVersion = "v0.53.0";
const remoteVersion = "v0.51.0";
const incompatibleHeading = "The sshc on bastion does not work with this computer’s";
const vaultPassword = "bastion vault password";
const sessionsPath = "/api/v1/engines/bastion/terminal/sessions";

// updateOutput is what the stubbed sshc on bastion writes while it updates, in
// the two parts the log answers: the first while it still runs, the rest once
// the test lets it end.
const updateOutput = [
  `sshc: update /home/me/.local/bin/sshc from ${remoteVersion} to ${localVersion} using sshc's install.sh\nsshc: updating ${remoteVersion} to ${localVersion}\n`,
  `sshc: updated to ${localVersion}\nsshc: managed service restarted\n`,
] as const;

// refusal is what the stubbed engine on bastion answers until it is fixed:
// an incompatible version, which updating fixes, a locked Vault, which its
// password unlocks, or a host that cannot be reached until the test says it can.
async function stubRunTarget(page: Page, { added = true, refusal = "version", compatibleVersion }: { added?: boolean; refusal?: "version" | "unreachable" | "locked"; compatibleVersion?: string } = {}) {
  let fixed = compatibleVersion !== undefined;
  const actions: unknown[] = [];
  let finishUpdate = () => {};
  const updateFinished = new Promise<void>((resolve) => { finishUpdate = resolve; });
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
      await route.fulfill({ status: 202, json: { generation: 1 } });
      return;
    }
    if (path.endsWith("/control/log")) {
      const cursor = Number(new URL(route.request().url()).searchParams.get("cursor") ?? "0");
      const [running, rest] = updateOutput;
      if (cursor < running.length) {
        await route.fulfill({ json: actionLog({ done: false, from: 0, output: running }) });
        return;
      }
      await updateFinished;
      fixed = true;
      await route.fulfill({ json: actionLog({ done: true, from: running.length, output: rest }) });
      return;
    }
    if (refusal === "locked" && path.endsWith("/vault/unlock")) {
      expect(route.request().postDataJSON()).toEqual({ passphrase: vaultPassword });
      fixed = true;
      await route.fulfill({ status: 204 });
      return;
    }
    if (refusal === "locked" && path.endsWith("/status")) {
      await route.fulfill({ json: { alias: "bastion", version: localVersion, vault: true, unlocked: fixed, passwordless: false, sessions: 0 } });
      return;
    }
    if (!fixed) {
      await route.fulfill(refusalResponse(refusal));
      return;
    }
    if (path.endsWith("/status")) {
      await route.fulfill({ json: { alias: "bastion", version: compatibleVersion ?? localVersion, vault: true, unlocked: true, passwordless: true, sessions: 0 } });
    } else if (path.endsWith("/terminal/sessions")) {
      await route.fulfill({ json: { sessions: [], maxSessions: 50 } });
    } else if (path.endsWith("/terminal/restore")) {
      // The engine on bastion has nothing to reopen.
      await route.fulfill({ json: { sessions: [] } });
    } else if (path.endsWith("/terminal/workspaces")) {
      await route.fulfill({ json: { workspaces: [{ id: "default", name: "", hidden: false, builtIn: true }], maxWorkspaces: 50 } });
    } else {
      throw new Error(`Unexpected remote engine request: ${path}`);
    }
  });
  return { actions, fix: () => { fixed = true; }, finishUpdate };
}

// actionLog is the log of the update on bastion from the cursor at from.
function actionLog({ done, from, output }: { done: boolean; from: number; output: string }) {
  const next = from + output.length;
  return {
    generation: 1, action: "update", done, succeeded: done,
    cursor: { requested: from, start: from, next, end: next, truncated: false },
    output,
  };
}

function refusalResponse(refusal: "version" | "unreachable" | "locked") {
  switch (refusal) {
    case "version":
      return { status: 409, contentType: "application/problem+json", json: { code: "remote_engine_incompatible", message: "request rejected", detail: remoteVersion } };
    case "locked":
      return { status: 423, contentType: "application/problem+json", json: { code: "remote_engine_vault_locked", message: "request rejected" } };
    case "unreachable":
      return { status: 502, contentType: "application/problem+json", json: { code: "remote_engine_unreachable", message: "request rejected" } };
  }
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
  await expect(page.getByRole("region").getByRole("button", { name: "localhost に戻す", exact: true })).toBeVisible();
  await page.screenshot({ path: `${directory}/${name}.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  // The sidebar slides away at this width; the screenshot waits until it is gone.
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  await page.screenshot({ path: `${directory}/${name}-mobile.png` });
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "en");
}

test("updates an incompatible engine from the terminal area and recovers", async ({ page, installation }) => {
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: incompatibleHeading, exact: true });
  await expect(panel).toBeVisible();
  await expect(panel.getByText(remoteVersion, { exact: true })).toBeVisible();
  await expect(panel.getByText(localVersion, { exact: true })).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-incompatible");

  await panel.getByRole("button", { name: "Update bastion", exact: true }).click();
  // The first lines show while sshc on bastion still runs.
  const output = panel.getByRole("log", { name: "Output of sshc on that host", exact: true });
  await expect(output).toContainText(`sshc: updating ${remoteVersion} to ${localVersion}`);
  await expect(panel.getByRole("button", { name: "Updating bastion…", exact: true })).toBeDisabled();
  await expect(output).not.toContainText("sshc: updated to");
  remote.finishUpdate();
  await expect(panel).toBeHidden();
  expect(remote.actions).toEqual([{ action: "update" }]);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

// Switching the language leaves the terminal screen, which would stop following
// the update, so this screenshot starts in Japanese instead of switching later.
test("shows the update's output in Japanese while sshc on the host runs", async ({ page, installation }) => {
  const directory = process.env.SSHC_VISUAL_DIR;
  test.skip(directory === undefined, "takes screenshots only when SSHC_VISUAL_DIR asks for them");
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.setViewportSize({ width: 1280, height: 800 });
  await changeDisplayLanguage(page, "ja");
  await page.getByRole("button", { name: /^ターミナルの実行先を選ぶ/ }).click();
  await page.getByRole("list", { name: "実行中のsshcエンジン", exact: true }).getByRole("button", { name: "bastion", exact: true }).click();
  const panel = page.getByRole("region", { name: "bastion のsshcとは組み合わせて使えません", exact: true });
  await panel.getByRole("button", { name: "bastion を更新", exact: true }).click();
  await expect(panel.getByRole("log", { name: "接続先の sshc の出力", exact: true })).toContainText("sshc: updating");
  await page.screenshot({ path: `${directory}/run-target-updating.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator("[data-app-navigation]")).not.toBeInViewport();
  await page.screenshot({ path: `${directory}/run-target-updating-mobile.png` });
  remote.finishUpdate();
  await expect(panel).toBeHidden();
});

test("unlocks a locked vault from the terminal area without a dialog", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "locked" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "The vault on bastion is locked", exact: true });
  const password = panel.getByLabel("Vault password for bastion", { exact: true });
  await expect(password).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await screenshotInJapanese(page, "run-target-locked");

  await password.fill(vaultPassword);
  await panel.getByRole("button", { name: "Unlock", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

test("shows an unreachable host in the terminal area and goes away on Reconnect now", async ({ page, installation }) => {
  const remote = await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "Could not connect to bastion", exact: true });
  await expect(panel.getByText("The engine terminals run on cannot be reached. Reconnect, or switch back to localhost.", { exact: true })).toBeVisible();
  await expect(panel.getByText("Tries again 2 s after each attempt ends", { exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("alert")).toHaveCount(0);
  await panel.getByRole("button", { name: "Details", exact: true }).click();
  await expect(panel.getByText("remote_engine_unreachable", { exact: true })).toBeVisible();
  await screenshotInJapanese(page, "run-target-unreachable");

  // The list is read again every two seconds; right after one reading the
  // next is still far off, so the panel can only go away through Reconnect now.
  await page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath);
  remote.fix();
  await Promise.all([
    page.waitForResponse((response) => new URL(response.url()).pathname === sessionsPath && response.ok()),
    panel.getByRole("button", { name: "Reconnect now", exact: true }).click(),
  ]);
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose where terminals run \(now: bastion\)/ })).toBeVisible();
});

test("switches back to localhost from the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "Could not connect to bastion", exact: true });
  await panel.getByRole("button", { name: "Switch back to localhost", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose where terminals run \(now: localhost\)/ })).toBeVisible();
});

test("keeps the run-target switcher usable while the panel is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "locked" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  const panel = page.getByRole("region", { name: "The vault on bastion is locked", exact: true });
  await expect(panel).toBeVisible();
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: /^localhost/ }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole("button", { name: /^Choose where terminals run \(now: localhost\)/ })).toBeVisible();
});

test("keeps the navigation reachable on a phone while the panel is shown", async ({ page, installation }) => {
  await stubRunTarget(page, { refusal: "unreachable" });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  await expect(page.getByRole("region", { name: "Could not connect to bastion", exact: true })).toBeVisible();
  // The panel makes the terminal inert, so a phone keeps the app header instead of the
  // terminal's own top bar, and the navigation (with the run-target switcher) stays one tap away.
  await page.setViewportSize({ width: 390, height: 844 });
  const openNavigation = page.getByRole("button", { name: "Navigation", exact: true });
  await expect(openNavigation).toBeVisible();
  await openNavigation.click();
  await expect(page.getByRole("button", { name: /^Choose where terminals run/ })).toBeVisible();
});

test("selects a newly added engine and shows it is incompatible in the terminal area", async ({ page, installation }) => {
  await stubRunTarget(page, { added: false });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Add an engine…", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "Add the engine on an SSH host", exact: true });
  await picker.getByRole("button", { name: /^bastion/ }).click();
  const panel = page.getByRole("region", { name: incompatibleHeading, exact: true });
  await expect(panel.getByRole("button", { name: "Update bastion", exact: true })).toBeVisible();
});

test("uses a compatible engine of another version without a panel", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: remoteVersion });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await chooseBastion(page);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
  await expect(page.getByRole("region", { name: incompatibleHeading, exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Actions for engine bastion", exact: true }).click();
  await page.getByRole("button", { name: "Engine information…", exact: true }).click();
  const information = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(information.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(information.getByText(`this computer runs ${localVersion}`, { exact: true })).toBeVisible();
});

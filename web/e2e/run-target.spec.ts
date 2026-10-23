import type { Page } from "@playwright/test";
import { expect, openApplication, test } from "./support/environment";

const localVersion = "v0.53.0";
const remoteVersion = "v0.51.0";
const incompatiblePrompt = "Cannot connect to bastion because its sshc version does not work with this computer’s. Update it?";

// stubRunTarget answers for an engine on bastion. Until it is updated it is
// refused as incompatible, unless compatibleVersion names the version of a
// compatible engine that answers from the start.
async function stubRunTarget(page: Page, { added = true, compatibleVersion }: { added?: boolean; compatibleVersion?: string } = {}) {
  let updated = compatibleVersion !== undefined;
  let listings = 0;
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
      updated = true;
      await route.fulfill({ json: { succeeded: true, output: "sshc updated" } });
      return;
    }
    if (path.endsWith("/terminal/sessions")) listings += 1;
    if (!updated) {
      await route.fulfill({ status: 409, contentType: "application/problem+json", json: {
        code: "remote_engine_incompatible", message: "request rejected", detail: remoteVersion,
      } });
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
  return { actions, listingCount: () => listings };
}

async function chooseBastion(page: Page) {
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("list", { name: "Engines", exact: true }).getByRole("button", { name: "bastion", exact: true }).click();
}

test("offers an update automatically, respects dismissal, and recovers after updating", async ({ page, installation }) => {
  const remote = await stubRunTarget(page);
  await openApplication(page, installation);
  await chooseBastion(page);
  const dialog = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(dialog.getByText(incompatiblePrompt, { exact: true })).toBeVisible();
  await expect(dialog.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(dialog.getByText(`this computer runs ${localVersion}`, { exact: true })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/incompatible-engine-dialog.png`, fullPage: true });
  }
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  const listingsBefore = remote.listingCount();
  await expect.poll(remote.listingCount).toBeGreaterThan(listingsBefore);
  await expect(dialog).toBeHidden();

  // Reselecting the current target must offer again even though its alias did not change.
  await chooseBastion(page);
  await expect(dialog.getByText(incompatiblePrompt, { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByRole("navigation", { name: "Primary", exact: true }).getByRole("button", { name: "Update…", exact: true }).click();
  await expect(dialog.getByText(incompatiblePrompt, { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Update", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "Update sshc on bastion?", exact: true });
  await confirmation.getByRole("button", { name: "Update", exact: true }).click();
  await expect(dialog.getByText(incompatiblePrompt, { exact: true })).toBeHidden();
  await expect(dialog.getByText(localVersion, { exact: true })).toBeVisible();
  expect(remote.actions).toEqual([{ action: "update" }]);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(page.getByText("Cannot connect to the run target’s sshc engine because its version does not work with this computer’s.", { exact: true })).toBeHidden();
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
});

test("selects a newly added engine and offers an update when it is incompatible", async ({ page, installation }) => {
  await stubRunTarget(page, { added: false });
  await openApplication(page, installation);
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Add an engine…", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "Add the engine on an SSH host", exact: true });
  await picker.getByRole("button", { name: /^bastion/ }).click();
  await expect(page.getByRole("dialog", { name: "Engine on bastion", exact: true }).getByText(incompatiblePrompt, { exact: true })).toBeVisible();
});

test("uses a compatible engine of another version without offering an update", async ({ page, installation }) => {
  await stubRunTarget(page, { compatibleVersion: remoteVersion });
  await openApplication(page, installation);
  await chooseBastion(page);
  await expect(page.getByRole("list", { name: "Workspaces", exact: true }).getByRole("button", { name: /^Main/ }).first()).toBeVisible();
  const dialog = page.getByRole("dialog", { name: "Engine on bastion", exact: true });
  await expect(dialog).toBeHidden();
  await page.getByRole("button", { name: /^Choose where terminals run/ }).click();
  await page.getByRole("button", { name: "Actions for engine bastion", exact: true }).click();
  await page.getByRole("button", { name: "Engine information…", exact: true }).click();
  await expect(dialog.getByRole("definition").filter({ hasText: remoteVersion })).toBeVisible();
  await expect(dialog.getByText(`this computer runs ${localVersion}`, { exact: true })).toBeVisible();
  await expect(dialog.getByText(incompatiblePrompt, { exact: true })).toBeHidden();
});

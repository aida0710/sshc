import { join } from "node:path";
import { expect, openApplication, openSection, openSettingsPage, test } from "./support/environment";

// The speed limit belongs to the preset, so each machine can choose its own; recovery after a lost connection
// is a global setting, the same on every machine.
test("saves the aggregate speed limit in the preset and bounded recovery in the global settings across a reload", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  await page.getByRole("button", { name: "Transfer settings", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/sftp$/);
  await expect(page.getByRole("heading", { name: "SFTP", exact: true })).toBeVisible();

  const speed = page.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await expect(speed).toHaveValue("0");
  const speedSaved = page.waitForResponse((response) =>
    new URL(response.url()).pathname === "/api/v1/settings/presets/default" && response.request().method() === "PUT");
  await speed.fill("128");
  await speed.press("Enter");
  expect((await speedSaved).status()).toBe(200);
  await expect(speed).toHaveValue("128");

  await openSettingsPage(page, "General");
  const globalSaved = () => page.waitForResponse((response) =>
    new URL(response.url()).pathname === "/api/v1/settings/global" && response.request().method() === "PUT");
  const recovery = page.getByRole("checkbox", { name: "Resume automatically", exact: true });
  const recoverySaved = globalSaved();
  await recovery.click();
  expect((await recoverySaved).status()).toBe(200);
  await expect(recovery).toBeChecked();
  const attempts = page.getByRole("spinbutton", { name: "Reconnect limit", exact: true });
  await expect(attempts).toHaveValue("3");
  const attemptsSaved = globalSaved();
  await attempts.fill("2");
  await attempts.press("Enter");
  expect((await attemptsSaved).status()).toBe(200);

  const saved = JSON.parse(await installation.read("sshc/metadata.json")) as {
    settings?: { fileTransfers?: { autoReconnect?: boolean; maxReconnectAttempts?: number } };
    presets: { fileTransfers?: { speedLimitBytesPerSecond?: number } }[];
  };
  expect(saved.presets[0]?.fileTransfers).toMatchObject({ speedLimitBytesPerSecond: 128 * 1024 });
  expect(saved.settings?.fileTransfers).toMatchObject({ autoReconnect: true, maxReconnectAttempts: 2 });
  await page.reload();
  await expect(recovery).toBeChecked();
  await expect(attempts).toHaveValue("2");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "transfer-recovery-settings-en.png"), fullPage: true });
  await openSettingsPage(page, "SFTP");
  await expect(speed).toHaveValue("128");
});

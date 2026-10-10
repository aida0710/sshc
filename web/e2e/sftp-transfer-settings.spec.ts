import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";

test("saves the aggregate speed limit and bounded recovery settings across a reload", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  await page.getByText("Transfer settings", { exact: true }).click();

  const speed = page.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await expect(speed).toHaveValue("0");
  const speedSaved = page.waitForResponse((response) => response.url().endsWith("/api/v1/sftp/transfers/settings") && response.request().method() === "PUT");
  await speed.fill("128");
  await speed.press("Enter");
  expect((await speedSaved).status()).toBe(200);
  await expect(speed).toHaveValue("128");

  const recovery = page.getByRole("checkbox", { name: "Recover after connection loss", exact: true });
  const recoverySaved = page.waitForResponse((response) => response.url().endsWith("/api/v1/sftp/transfers/settings") && response.request().method() === "PUT");
  await recovery.click();
  expect((await recoverySaved).status()).toBe(200);
  await expect(recovery).toBeChecked();
  const attempts = page.getByRole("spinbutton", { name: "Maximum reconnect attempts", exact: true });
  await expect(attempts).toHaveValue("3");
  const attemptsSaved = page.waitForResponse((response) => response.url().endsWith("/api/v1/sftp/transfers/settings") && response.request().method() === "PUT");
  await attempts.fill("2");
  await attempts.press("Enter");
  expect((await attemptsSaved).status()).toBe(200);

  const saved = JSON.parse(await installation.read("sshc/metadata.json")) as {
    fileTransfers: { speedLimitBytesPerSecond: number; autoReconnect: boolean; maxReconnectAttempts: number };
  };
  expect(saved.fileTransfers).toMatchObject({
    speedLimitBytesPerSecond: 128 * 1024,
    autoReconnect: true,
    maxReconnectAttempts: 2,
  });
  await page.reload();
  await page.getByText("Transfer settings", { exact: true }).click();
  await expect(speed).toHaveValue("128");
  await expect(recovery).toBeChecked();
  await expect(attempts).toHaveValue("2");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "transfer-recovery-settings-en.png"), fullPage: true });
});

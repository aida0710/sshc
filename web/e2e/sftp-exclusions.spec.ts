import { expect, openApplication, openSection, test } from "./support/environment";

test("saves folder exclusion rules and preserves them while changing other transfer settings", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  await page.getByText("Transfer settings", { exact: true }).click();
  await page.getByText("Transfer exclusions", { exact: true }).click();
  const patterns = page.getByRole("textbox", { name: "Exclusion patterns (one per line)", exact: true });
  await patterns.fill(".git\nnode_modules\n*.log");
  const exclusionsSaved = page.waitForResponse((response) => response.url().endsWith("/api/v1/sftp/transfers/settings") && response.request().method() === "PUT");
  await page.getByRole("button", { name: "Save exclusions", exact: true }).click();
  expect((await exclusionsSaved).status()).toBe(200);
  await expect(page.getByText("3 exclusion rules", { exact: true })).toBeVisible();

  const speedSaved = page.waitForResponse((response) => response.url().endsWith("/api/v1/sftp/transfers/settings") && response.request().method() === "PUT");
  const speed = page.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await speed.fill("128");
  await speed.press("Enter");
  expect((await speedSaved).status()).toBe(200);
  const metadata = JSON.parse(await installation.read("sshc/metadata.json")) as {
    fileTransfers: { excludePatterns: string[]; speedLimitBytesPerSecond: number };
  };
  expect(metadata.fileTransfers.excludePatterns).toEqual([".git", "node_modules", "*.log"]);
  expect(metadata.fileTransfers.speedLimitBytesPerSecond).toBe(128 * 1024);

  await page.reload();
  await page.getByText("Transfer settings", { exact: true }).click();
  await expect(page.getByText("3 exclusion rules", { exact: true })).toBeVisible();
  await page.getByText("3 exclusion rules", { exact: true }).click();
  await expect(patterns).toHaveValue(".git\nnode_modules\n*.log");
  await patterns.fill("../secret");
  await expect(page.getByRole("button", { name: "Save exclusions", exact: true })).toBeDisabled();
  await expect(page.getByRole("alert")).toContainText("Check the patterns");
});

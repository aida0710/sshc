import { expect, openApplication, openSection, test } from "./support/environment";

test("saves folder exclusion rules and preserves them while changing other transfer settings", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  await page.getByRole("button", { name: "Transfer settings", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/sftp$/);
  const patterns = page.getByRole("textbox", { name: "Exclusion patterns (one per line)", exact: true });
  await patterns.fill(".git\nnode_modules\n*.log");
  const exclusionsSaved = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/settings/presets/default" && response.request().method() === "PUT");
  // The rules are saved when their box is left.
  await patterns.press("Tab");
  expect((await exclusionsSaved).status()).toBe(200);

  const speedSaved = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/settings/presets/default" && response.request().method() === "PUT");
  const speed = page.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await speed.fill("128");
  await speed.press("Enter");
  expect((await speedSaved).status()).toBe(200);
  const metadata = JSON.parse(await installation.read("sshc/metadata.json")) as {
    presets: { fileTransfers?: { excludePatterns?: string[]; speedLimitBytesPerSecond?: number } }[];
  };
  expect(metadata.presets[0]?.fileTransfers?.excludePatterns).toEqual([".git", "node_modules", "*.log"]);
  expect(metadata.presets[0]?.fileTransfers?.speedLimitBytesPerSecond).toBe(128 * 1024);

  await page.reload();
  await expect(patterns).toHaveValue(".git\nnode_modules\n*.log");
  // A rule that is not valid is not saved when the box is left.
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "PUT" && new URL(request.url()).pathname.startsWith("/api/v1/settings/")) writes.push(request.url());
  });
  await patterns.fill("../secret");
  await patterns.press("Tab");
  await expect(patterns).toHaveAccessibleDescription(/Check the patterns/);
  expect(writes).toEqual([]);
});

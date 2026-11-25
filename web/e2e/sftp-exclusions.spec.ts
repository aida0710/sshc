import { codeField, codeFieldLines, replaceCodeFieldText } from "./support/codeField";
import { expect, openApplication, openSection, test } from "./support/environment";
import { saveButton, saveSettings } from "./support/saveBar";

test("saves folder exclusion rules and preserves them while changing other transfer settings", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1400, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  await page.getByRole("button", { name: "Transfer settings", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/sftp$/);
  const patterns = page.getByRole("textbox", { name: "Exclusion patterns (one per line)", exact: true });
  const patternsField = codeField(page, "Exclusion patterns (one per line)");
  await replaceCodeFieldText(patternsField, ".git\nnode_modules\n*.log");
  await saveSettings(page);

  const speedSaved = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/settings/presets/default" && response.request().method() === "PUT");
  const speed = page.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await speed.fill("128");
  // Enter in a one-line field saves as the button does.
  await speed.press("Enter");
  expect((await speedSaved).status()).toBe(200);
  const metadata = JSON.parse(await installation.read("sshc/metadata.json")) as {
    presets: { fileTransfers?: { excludePatterns?: string[]; speedLimitBytesPerSecond?: number } }[];
  };
  expect(metadata.presets[0]?.fileTransfers?.excludePatterns).toEqual([".git", "node_modules", "*.log"]);
  expect(metadata.presets[0]?.fileTransfers?.speedLimitBytesPerSecond).toBe(128 * 1024);

  await page.reload();
  await expect(codeFieldLines(patternsField)).toHaveText(".git\nnode_modules\n*.log".replaceAll("\n", ""));
  // A rule that is not valid cannot be saved.
  await replaceCodeFieldText(patternsField, "../secret");
  await expect(patterns).toHaveAccessibleDescription(/Check the patterns/);
  await expect(saveButton(page)).toBeDisabled();
});

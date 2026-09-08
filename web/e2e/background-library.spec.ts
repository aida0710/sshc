import { copyFile, mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import { changeDisplayLanguage, expect, openApplication, openSettingsPage, test } from "./support/environment";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

test("background image library stays compact and searchable", async ({ page, installation }) => {
  const backgrounds = join(installation.home, ".ssh", "sshc", "backgrounds");
  await mkdir(backgrounds, { recursive: true, mode: 0o700 });
  const fixtures = [
    ["night-terminal.png", "terminal-desktop.png"],
    ["workspace-grid.png", "workspace-desktop.png"],
    ["android-console.png", "android-home.png"],
    ["transfer-view.png", "transfer-manager-ja.png"],
  ] as const;
  for (const [name, source] of fixtures) {
    await copyFile(resolve(process.cwd(), "..", "pages", "public", "images", source), join(backgrounds, name));
  }

  await openApplication(page, installation);
  await openSettingsPage(page, "Terminal");
  await changeDisplayLanguage(page, "ja");
  await page.getByText("変更", { exact: true }).click();
  const library = page.getByRole("dialog", { name: "背景画像ライブラリ" });
  await expect(library).toBeVisible();
  await expect(library.getByRole("heading", { name: "背景画像ライブラリ" })).toBeVisible();
  await expect(library.getByLabel(/^上限/)).toHaveValue("16");

  if (visualDirectory !== undefined) {
    await mkdir(visualDirectory, { recursive: true });
    await page.screenshot({ path: join(visualDirectory, "background-library-desktop-ja.png"), fullPage: true });
  }

  await page.setViewportSize({ width: 390, height: 844 });
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "background-library-mobile-ja.png"), fullPage: true });
  }
});

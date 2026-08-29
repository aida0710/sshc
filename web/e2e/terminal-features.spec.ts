import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

test("keeps terminal actions compact and exposes the new terminal settings", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "Local shell" }).click();

  const terminal = page.getByRole("region", { name: /^Console for / });
  await expect(terminal).toBeVisible();
  await expect(terminal.getByRole("button", { name: "Find" })).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  const menu = terminal.getByRole("menu", { name: "More terminal actions" });
  await expect(menu.getByRole("menuitem", { name: "Quick Commands" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Copy recent terminal context" })).toBeVisible();
  await expect(menu.getByRole("menuitemcheckbox", { name: "OSC 52" })).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-desktop.png"), fullPage: true });
  }

  await page.keyboard.press("Escape");
  await openSection(page, "Settings");
  const settings = page.getByRole("region", { name: "Terminal" });
  await expect(settings.getByLabel("Browser scrollback (lines)")).toBeVisible();
  await expect(settings.getByLabel("Default local shell")).toBeVisible();
  await expect(settings.getByText("Allow OSC 52 clipboard writes by default")).toBeVisible();
  await expect(settings.getByText("Send the JIS ¥ key as backslash")).toBeVisible();

  if (visualDirectory !== undefined) {
    await settings.getByLabel("Browser scrollback (lines)").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-settings-desktop.png"), fullPage: true });
    await settings.getByText("Send the JIS ¥ key as backslash").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-input-settings-desktop.png"), fullPage: true });
  }

  await page.setViewportSize({ width: 360, height: 800 });
  await page.reload();
  await openSection(page, "Terminal");
  await expect(terminal).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  await expect(menu).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-mobile.png"), fullPage: true });
  }
});

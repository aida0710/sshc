import { expect, test } from "./support/environment";
import { openApplication, openSection } from "./support/environment";

test("searches hosts, config files and settings from the keyboard", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", "Host r540\n  HostName 192.0.2.54\n  User aida\n");
  await openApplication(page, installation);
  // snippets.json is sealed with the Vault key, so the snippet is saved the way a person saves one.
  await openSection(page, "Snippets");
  await page.getByRole("button", { name: "Add snippet", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Update packages");
  await page.getByLabel("Command", { exact: true }).fill("sudo apt update");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("Snippet saved.", { exact: true })).toBeVisible();

  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" });
  await expect(palette).toBeVisible();

  const search = palette.getByRole("searchbox");
  await search.fill("r540");
  await expect(palette.getByRole("option", { name: /Connect to r540/ })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/command-palette-host-settings.png`, fullPage: true });
  }
  await palette.getByRole("button", { name: "Open connection settings for r540" }).click();
  await expect(page).toHaveURL(/\/connections\/servers\?path=conf\.d%2F20-lab\.conf&host=r540&panel=basic$/);
  await expect(page.getByRole("heading", { name: "r540", exact: true })).toBeVisible();
  await expect(palette).toBeHidden();

  await page.keyboard.press("Control+k");
  await search.fill("config");
  await expect(palette.getByRole("option", { name: /conf\.d\/20-lab\.conf/ })).toBeVisible();

  await search.fill("apt update");
  await palette.getByRole("option", { name: /Update packages/ }).click();
  await expect(page).toHaveURL(/\/snippets\?snippet=[0-9a-f]{32}$/);
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Update packages");

  await page.keyboard.press("Control+k");
  await search.fill("sync");
  await palette.getByRole("option", { name: /Sync/ }).click();
  await expect(page).toHaveURL(/\/sync$/);
  await expect(palette).toBeHidden();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.keyboard.press("Control+k");
    await expect(page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" })).toBeVisible();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.0-command-palette-desktop.png`, fullPage: true });
  }
});

test("opens the palette from the desktop toolbar", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Search everything" }).click();
  await expect(page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" })).toBeVisible();
});

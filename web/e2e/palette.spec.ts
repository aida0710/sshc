import { expect, test } from "./support/environment";
import { openApplication } from "./support/environment";

test("searches hosts, config files and settings from the keyboard", async ({ page, installation }) => {
  await installation.write("conf.d/20-lab.conf", "Host r540\n  HostName 192.0.2.54\n  User aida\n");
  await installation.write("sshc/snippets.json", JSON.stringify({
    schemaVersion: 1,
    snippets: [{
      id: "0123456789abcdef0123456789abcdef",
      name: "Update packages",
      description: "Refresh package indexes",
      command: "sudo apt update",
      variables: [],
      createdAt: "2026-08-27T00:00:00Z",
      updatedAt: "2026-08-27T00:00:00Z",
    }],
  }));
  await openApplication(page, installation);

  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Search hosts, files, snippets and settings" });
  await expect(palette).toBeVisible();

  const search = palette.getByRole("searchbox");
  await search.fill("r540");
  await expect(palette.getByRole("option", { name: /Connect to r540/ })).toBeVisible();

  await search.fill("config");
  await expect(palette.getByRole("option", { name: /conf\.d\/20-lab\.conf/ })).toBeVisible();

  await search.fill("apt update");
  await palette.getByRole("option", { name: /Update packages/ }).click();
  await expect(page).toHaveURL(/\/snippets\?snippet=0123456789abcdef0123456789abcdef$/);
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Update packages");

  await page.keyboard.press("Control+k");
  await search.fill("sync");
  await palette.getByRole("option", { name: /Sync/ }).click();
  await expect(page).toHaveURL(/\/sync$/);
  await expect(palette).toBeHidden();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.keyboard.press("Control+k");
    await expect(page.getByRole("dialog", { name: "Search hosts, files, snippets and settings" })).toBeVisible();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.0-command-palette-desktop.png`, fullPage: true });
  }
});

test("opens the palette from the desktop toolbar", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Search everything" }).click();
  await expect(page.getByRole("dialog", { name: "Search hosts, files, snippets and settings" })).toBeVisible();
});

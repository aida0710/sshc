import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import type { RemoteEntry } from "../src/sftp/api";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";
import { connectSFTPHost, openSecondSFTPPane } from "./support/sftp";

// Fixture paths and metadata let us inspect destructive controls without a live SSH connection.
const deletionEntries: RemoteEntry[] = [
  { name: "old-project", path: "/work/old-project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-10-07T06:00:00Z", revision: "folder-v1" },
  { name: "notes.txt", path: "/work/notes.txt", type: "file", size: 2048, mode: "0644", modifiedAt: "2026-10-07T06:00:00Z", revision: "file-v1" },
  { name: "current", path: "/work/current", type: "symlink", size: 0, mode: "0777", modifiedAt: "2026-10-07T06:00:00Z", revision: "link-v1", linkTarget: "old-project", targetType: "directory" },
];

const deletionLayouts = [
  { name: "desktop", width: 1280, height: 800, secondPane: false, mobile: false },
  { name: "compact", width: 1280, height: 800, secondPane: true, mobile: false },
  { name: "mobile", width: 390, height: 800, secondPane: false, mobile: true },
  { name: "narrow", width: 360, height: 800, secondPane: false, mobile: true },
];

for (const layout of deletionLayouts) {
  test(`offers confirmed deletion of files, folders and multiple entries in the ${layout.name} layout`, async ({ page, installation }) => {
    await page.setViewportSize({ width: layout.width, height: layout.height });
    const deletionRequests: string[] = [];
    await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({ json: { path: "/work", entries: deletionEntries } }));
    await page.route("**/api/v1/sftp/transfers", async (route) => {
      if (route.request().method() === "GET") {
        await route.continue();
        return;
      }
      deletionRequests.push(route.request().postData() ?? "");
      await route.abort();
    });
    await openApplication(page, installation);
    if (layout.mobile) await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "SFTP", exact: true }).click();
    else await openSection(page, "SFTP");
    await connectSFTPHost(page, "bastion");
    if (layout.secondPane) await openSecondSFTPPane(page, "New tab");
    const pane = page.getByRole("tabpanel").first();

    for (const entry of deletionEntries) {
      await pane.getByRole("checkbox", { name: `Select ${entry.name}`, exact: true }).check();
      await expect(pane.getByRole("button", { name: "Delete", exact: true })).toHaveCount(0);
      const actions = pane.getByRole("button", { name: `Actions for ${entry.name}`, exact: true });
      await expect(actions).toBeVisible();
      if (layout.mobile) expect((await actions.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await actions.click();
      await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "Delete this remote entry?", exact: true });
      await expect(dialog).toContainText(entry.path);
      if (entry.type === "directory") await expect(dialog).toContainText("Folders and everything inside them will be deleted.");
      else await expect(dialog).not.toContainText("Folders and everything inside them will be deleted.");
      await expect(dialog.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await pane.getByRole("button", { name: "Clear selection", exact: true }).click();
    }

    for (const entry of deletionEntries) await pane.getByRole("checkbox", { name: `Select ${entry.name}`, exact: true }).check();
    await pane.getByRole("button", { name: "Actions for 3 selected items", exact: true }).click();
    await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Delete 3 remote entries?", exact: true });
    for (const entry of deletionEntries) await expect(dialog).toContainText(entry.path);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();

    const visualDirectory = process.env.SSHC_VISUAL_DIR;
    if (visualDirectory !== undefined) {
      await mkdir(visualDirectory, { recursive: true });
      await changeDisplayLanguage(page, "ja");
      await connectSFTPHost(page, "bastion", pane);
      for (const entry of deletionEntries) await pane.getByRole("checkbox", { name: `${entry.name}を選択`, exact: true }).check();
      await expect(pane.getByRole("button", { name: "削除", exact: true })).toHaveCount(0);
      await pane.getByRole("button", { name: "選択した3件の操作", exact: true }).click();
      await expect(page.getByRole("menuitem", { name: "削除", exact: true })).toBeVisible();
      await page.screenshot({ path: join(visualDirectory, `${layout.name}-menu-ja.png`), animations: "disabled" });
      await page.getByRole("menuitem", { name: "削除", exact: true }).click();
      await expect(page.getByRole("dialog")).toContainText("フォルダ内の項目もすべて削除されます。");
      await page.screenshot({ path: join(visualDirectory, `${layout.name}-confirmation-ja.png`), animations: "disabled" });
      await page.getByRole("dialog").getByRole("button", { name: "キャンセル", exact: true }).click();
    }
    expect(deletionRequests).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(layout.width);
  });
}

test("offers no remote deletion controls for engine-local files", async ({ page, installation }) => {
  await page.route("**/api/v1/sftp/local/entries**", (route) => route.fulfill({ json: { home: "/home/fixture", path: "/home/fixture", entries: [
    { ...deletionEntries[1], path: "/home/fixture/notes.txt" },
  ] } }));
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await pane.locator("button[data-value]:visible").click();
  await page.getByRole("dialog").getByText("Local", { exact: true }).click();
  await pane.getByRole("checkbox", { name: "Select notes.txt", exact: true }).check();
  await expect(pane.getByRole("button", { name: "Delete", exact: true })).toHaveCount(0);
  await pane.getByRole("button", { name: "Actions for notes.txt", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Delete", exact: true })).toHaveCount(0);
});

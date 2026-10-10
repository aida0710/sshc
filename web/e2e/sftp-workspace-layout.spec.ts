import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";
import { connectSFTPHost, openLocalSFTPDirectory, openSecondSFTPPane } from "./support/sftp";

// Mouse and touch controls must align without sacrificing phone tap targets.
for (const hasTouch of [false, true]) {
  test.describe(hasTouch ? "touch controls" : "mouse controls", () => {
    test.use({ hasTouch });

    test("aligns local and remote controls and keeps the file list usable", async ({ page, installation }) => {
      await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({ json: {
        path: "/srv", entries: [{ name: "remote.txt", path: "/srv/remote.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-10-10T00:00:00Z", revision: "remote-file" }],
      } }));
      await installation.write("layout/alpha.txt", "alpha");
      await installation.write("layout/beta.txt", "beta");
      await installation.write("layout/notes.txt", "notes");
      await page.setViewportSize({ width: 1440, height: 900 });
      await openApplication(page, installation);
      await openSection(page, "SFTP");
      await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: join(installation.home, ".ssh/layout") });
      await openSecondSFTPPane(page, "New tab");
      const remote = page.getByRole("tabpanel").nth(1);
      await connectSFTPHost(page, "bastion", remote);
      const local = page.getByRole("tabpanel").first();
      const localFilter = local.getByRole("searchbox", { name: "Filter entries" });
      const remoteFilter = remote.getByRole("searchbox", { name: "Filter entries" });
      await expect(remote.getByRole("button", { name: "remote.txt", exact: true })).toBeVisible();
      await expect.poll(async () => Math.abs((await localFilter.boundingBox())!.y - (await remoteFilter.boundingBox())!.y)).toBeLessThanOrEqual(1);
      await expect.poll(async () => Math.abs((await local.getByTestId("sftp-file-list").boundingBox())!.y - (await remote.getByTestId("sftp-file-list").boundingBox())!.y)).toBeLessThanOrEqual(1);
      await expect(local.getByRole("button", { name: "Host", exact: true })).toHaveCount(0);
      await expect(remote.getByRole("button", { name: "Host", exact: true })).toHaveCount(0);
      await expect(page.getByRole("combobox", { name: "Search mode" })).toHaveCount(0);
      // Split panes prioritize names and use the same columns; full metadata is in Details.
      await expect(local.getByRole("columnheader")).toHaveCount(5);
      await expect(remote.getByRole("columnheader")).toHaveCount(5);
      for (const pane of [local, remote]) {
        await expect.poll(() => pane.getByTestId("sftp-file-list").evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
      }
      for (const group of await page.locator("[data-sftp-pane-tabs]").all()) {
        const bounds = (await group.boundingBox())!;
        const selected = (await group.getByRole("tab", { selected: true }).boundingBox())!;
        expect(selected.y + selected.height).toBeLessThanOrEqual(bounds.y + bounds.height);
      }
      await remote.getByRole("button", { name: "Search mode", exact: true }).click();
      const options = page.getByRole("dialog", { name: "Search mode", exact: true });
      await options.getByRole("combobox", { name: "Search mode" }).selectOption("content");
      await options.getByRole("button", { name: "Close", exact: true }).click();
      await expect(remote.getByRole("searchbox", { name: "Text to find in files" })).toBeVisible();

      for (const viewport of hasTouch ? [{ width: 390, height: 640 }, { width: 844, height: 390 }] : []) {
        await page.setViewportSize(viewport);
        await page.getByRole("tablist", { name: "File tabs", exact: true }).getByRole("tab", { name: "Local:layout", exact: true }).click();
        const pane = page.getByRole("tabpanel");
        await expect(pane).toHaveCount(1);
        const list = pane.getByTestId("sftp-file-list");
        // Two complete file rows must fit even in landscape.
        await expect.poll(async () => (await list.boundingBox())!.height).toBeGreaterThanOrEqual(112);
        await expect.poll(async () => {
          const bounds = (await list.boundingBox())!;
          const rows = await Promise.all(["alpha.txt", "beta.txt"].map((name) => list.getByRole("button", { name, exact: true }).boundingBox()));
          return rows.every((row) => row !== null && row.y >= bounds.y && row.y + row.height <= bounds.y + bounds.height);
        }).toBe(true);
        const before = (await list.boundingBox())!.height;
        await pane.getByRole("checkbox", { name: "Select alpha.txt", exact: true }).check();
        await expect(pane.getByRole("button", { name: "Actions for alpha.txt", exact: true })).toBeVisible();
        await expect.poll(async () => Math.abs((await list.boundingBox())!.height - before)).toBeLessThanOrEqual(1);
        await pane.getByRole("button", { name: "Clear selection", exact: true }).click();
      }
    });
  });
}

test("opens a local shell in the displayed directory from the SFTP terminal button", async ({ page, installation }) => {
  await installation.write("shell-start/notes.txt", "notes");
  const directory = join(installation.home, ".ssh/shell-start");
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await openLocalSFTPDirectory({ page, pane, directory });
  const creation = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/terminal/sessions" && response.request().method() === "POST");
  await pane.getByRole("button", { name: "Open Terminal here", exact: true }).click();
  const response = await creation;
  expect(response.ok()).toBe(true);
  expect(response.request().postDataJSON()).toMatchObject({ kind: "shell", cwd: directory });
  expect(response.request().postDataJSON()).not.toHaveProperty("alias");
  await expect(page).toHaveURL(/\/terminal$/);
});

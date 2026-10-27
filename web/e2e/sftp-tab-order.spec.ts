import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openApplication, openSection, test } from "./support/environment";
import { openLocalSFTPDirectory } from "./support/sftp";

// tabNames lists a strip's tabs in the order they are shown.
async function tabNames(page: Page, strip: number): Promise<string[]> {
  return page.locator("[data-sftp-pane-tabs]").nth(strip).getByRole("tab").evaluateAll(
    (tabs) => tabs.map((tab) => tab.getAttribute("aria-label") ?? ""),
  );
}

async function chooseTabAction(page: Page, strip: number, item: string) {
  const selected = page.locator("[data-sftp-pane-tabs]").nth(strip).getByRole("tab", { selected: true });
  await selected.click({ button: "right" });
  await page.getByRole("menuitem", { name: item, exact: true }).click();
}

test("reorders SFTP tabs by dragging and from their menu, and swaps the two panes", async ({ page, installation }) => {
  let listings = 0;
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => {
    listings += 1;
    return route.fulfill({ json: {
      path: "/srv", entries: [{ name: "remote.txt", path: "/srv/remote.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-10-10T00:00:00Z", revision: "remote-file" }],
    } });
  });
  await installation.write("layout/alpha.txt", "alpha");
  await page.setViewportSize({ width: 1440, height: 900 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: join(installation.home, ".ssh/layout") });
  const strip = page.locator("[data-sftp-pane-tabs]").first();
  await strip.getByRole("button", { name: "New tab", exact: true }).click();
  await page.getByRole("dialog").getByText("bastion", { exact: true }).click();
  await expect(page.getByRole("tabpanel").getByRole("button", { name: "remote.txt", exact: true })).toBeVisible();
  await expect.poll(() => tabNames(page, 0)).toEqual(["Local:layout", "bastion:srv"]);

  // Dragged onto the left half of the first tab, the second tab goes before it.
  const first = strip.getByRole("tab", { name: "Local:layout", exact: true });
  await strip.getByRole("tab", { name: "bastion:srv", exact: true }).dragTo(first, { targetPosition: { x: 10, y: 10 } });
  await expect.poll(() => tabNames(page, 0)).toEqual(["bastion:srv", "Local:layout"]);

  // From the menu, and without reconnecting: the tab stays mounted in its pane.
  const before = listings;
  await strip.getByRole("tab", { name: "bastion:srv", exact: true }).click();
  await chooseTabAction(page, 0, "Move right");
  await expect.poll(() => tabNames(page, 0)).toEqual(["Local:layout", "bastion:srv"]);
  expect(listings).toBe(before);

  await chooseTabAction(page, 0, "Open in a new right pane");
  await expect(page.locator("[data-sftp-pane-tabs]")).toHaveCount(2);
  await expect.poll(() => tabNames(page, 1)).toEqual(["bastion:srv"]);

  const afterSplit = listings;
  await chooseTabAction(page, 1, "Swap the left and right panes");
  await expect.poll(() => tabNames(page, 0)).toEqual(["bastion:srv"]);
  await expect.poll(() => tabNames(page, 1)).toEqual(["Local:layout"]);
  await expect(page.getByRole("tabpanel").first().getByRole("button", { name: "remote.txt", exact: true })).toBeVisible();
  expect(listings).toBe(afterSplit);
});

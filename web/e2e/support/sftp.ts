import { expect, type Page } from "@playwright/test";

// Opens the second SFTP pane the way a user does: a blank tab is added to the
// left pane and dragged onto the right half of that pane, which moves the tab
// into a new pane on the right. `newTabLabel` is the "+" button's name in the
// language the page is showing.
export async function openSecondSFTPPane(page: Page, newTabLabel: string): Promise<void> {
  const leftStrip = page.locator("[data-sftp-pane-tabs]").first();
  await leftStrip.getByRole("button", { name: newTabLabel, exact: true }).click();
  const content = page.locator("[data-sftp-pane-content]").first();
  const bounds = await content.boundingBox();
  if (bounds === null) throw new Error("the SFTP pane is not visible");
  await leftStrip.getByRole("tab", { selected: true }).dragTo(content, {
    targetPosition: { x: Math.round(bounds.width * 0.75), y: Math.round(bounds.height / 2) },
  });
  await expect(page.locator("[data-sftp-pane-tabs]")).toHaveCount(2);
}

// Moves the selected tab of the right pane onto the left pane. When it was the
// right pane's only tab, the workspace is back to one pane.
export async function moveRightSFTPTabLeft(page: Page): Promise<void> {
  const rightStrip = page.locator("[data-sftp-pane-tabs]").nth(1);
  const content = page.locator("[data-sftp-pane-content]").first();
  await rightStrip.getByRole("tab", { selected: true }).dragTo(content);
}

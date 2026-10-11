import { expect, type Locator, type Page } from "@playwright/test";

export async function sftpHostPicker(page: Page, pane: Locator): Promise<Locator> {
  const inline = pane.locator("button[data-value]:visible");
  if (await inline.count() > 0) return inline;
  const paneContainer = pane.locator("xpath=ancestor-or-self::*[@data-sftp-pane]");
  const tabTrigger = paneContainer.locator("button[data-value]:visible");
  if (await tabTrigger.count() > 0) return tabTrigger;
  // Phones share one tab strip outside the visible pane.
  return page.locator("[data-sftp-pane-tabs] button[data-value]:visible");
}

// Connects a pane to `alias` the way a user does: opens the host picker and
// chooses the host, which connects. `pane` defaults to the only visible pane.
export async function connectSFTPHost(page: Page, alias: string, pane: Locator = page.getByRole("tabpanel")): Promise<void> {
  await (await sftpHostPicker(page, pane)).click();
  await page.getByRole("dialog").getByText(alias, { exact: true }).click();
}

export async function openLocalSFTPDirectory({ page, pane, directory }: {
  page: Page;
  pane: Locator;
  directory: string;
}): Promise<void> {
  await (await sftpHostPicker(page, pane)).click();
  await page.getByRole("dialog").getByText("Local", { exact: true }).click();
  await pane.getByRole("button", { name: "Edit local path", exact: true }).click();
  const pathInput = pane.getByRole("textbox", { name: "Engine filesystem path", exact: true });
  await pathInput.fill(directory);
  await pathInput.press("Enter");
}

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

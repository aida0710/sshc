import type { Page } from "@playwright/test";

// chooseConnectionAction picks one of the open connection's actions from the 「…」 in its header.
export async function chooseConnectionAction(page: Page, action: string | RegExp): Promise<void> {
  await page.locator("[data-connection-summary]").getByRole("button", { name: "Actions for this connection" }).click();
  await page.getByRole("menuitem", { name: action }).click();
}

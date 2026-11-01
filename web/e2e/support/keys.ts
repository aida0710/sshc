import type { Page } from "@playwright/test";
import { expect } from "./environment";

// openKeyCreation opens the form to create a key from the Keys page's heading, unless it is open already.
// The form stays open after a key is made, so a test creating several keys opens it once.
export async function openKeyCreation(page: Page) {
  const button = page.getByRole("button", { name: "Create a key", exact: true });
  if ((await button.getAttribute("aria-expanded")) !== "true") await button.click();
  await expect(page.getByLabel("File name")).toBeVisible();
}

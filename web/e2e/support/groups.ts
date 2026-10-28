import type { Locator, Page } from "@playwright/test";
import { expect, openSection } from "./environment";

// The list of groups beside the Connections card.
export function groupList(page: Page): Locator {
  return page.getByRole("navigation", { name: "Groups" });
}

// openGroupMenu opens the … of a group in that list.
export async function openGroupMenu(page: Page, name: string): Promise<void> {
  const groups = groupList(page);
  await groups.getByRole("button", { name, exact: true }).hover();
  await groups.getByRole("button", { name: `Actions for ${name}`, exact: true }).click();
}

// createGroups makes groups the way a person does in Connections: New group for a group at the top, and
// New group inside on the parent for a nested one. Parents come before their children in names.
export async function createGroups(page: Page, names: readonly string[]): Promise<void> {
  await openSection(page, "Connections");
  for (const name of names) {
    const slash = name.lastIndexOf("/");
    const parent = slash < 0 ? "" : name.slice(0, slash);
    if (parent === "") {
      await groupList(page).getByRole("button", { name: "New group", exact: true }).click();
    } else {
      await openGroupMenu(page, parent);
      await page.getByRole("menuitem", { name: "New group inside" }).click();
    }
    const dialog = page.getByRole("dialog", { name: parent === "" ? "New group" : `New group inside ${parent}` });
    await dialog.getByLabel("Name", { exact: true }).fill(name.slice(slash + 1));
    const [response] = await Promise.all([
      page.waitForResponse((candidate) => candidate.url().includes("/api/v1/config/save") && candidate.request().method() === "POST"),
      dialog.getByRole("button", { name: "Create", exact: true }).click(),
    ]);
    expect(response.status()).toBe(200);
    await expect(dialog).toBeHidden();
  }
}

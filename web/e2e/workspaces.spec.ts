import { expect, test, openApplication } from "./support/environment";

test("groups sessions into workspaces and keeps a hidden one out of the tree", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const tree = navigation.getByRole("list", { name: "Workspaces" });
  await expect(tree.getByRole("button", { name: /^Main/ })).toBeVisible();
  await expect(tree.getByRole("button", { name: /^Agents/ })).toHaveCount(0);

  await navigation.getByRole("button", { name: "Workspace list actions" }).click();
  await page.getByRole("menuitem", { name: "New workspace" }).click();
  await page.getByRole("textbox", { name: "Workspace name" }).fill("sshc dev");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const workspace = tree.getByRole("button", { name: /^sshc dev/ });
  await expect(workspace).toBeVisible();

  await workspace.hover();
  await navigation.getByRole("button", { name: "Open a session in sshc dev" }).click();
  const dialog = page.getByRole("dialog", { name: "New session" });
  await dialog.getByRole("button", { name: /^Local shell, localhost/ }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  await expect(workspace).toContainText("1");

  const session = tree.getByRole("button", { name: /^Actions for / });
  await session.click();
  await page.getByRole("menuitem", { name: "Agents" }).click();
  await expect(workspace).toContainText("0");
  await expect(tree.getByRole("button", { name: /^Actions for / })).toHaveCount(0);

  await navigation.getByRole("button", { name: "Workspace list actions" }).click();
  await page.getByRole("menuitem", { name: "Show hidden workspaces" }).click();
  await expect(tree.getByRole("button", { name: /^Agents/ })).toContainText("1");
  await expect(tree.getByRole("button", { name: /^Actions for / })).toHaveCount(1);

  await workspace.hover();
  await navigation.getByRole("button", { name: "Workspace actions for sshc dev" }).click();
  await page.getByRole("menuitem", { name: "Delete workspace" }).click();
  await page.getByRole("dialog", { name: "Delete sshc dev?" }).getByRole("button", { name: "Delete" }).click();
  await expect(tree.getByRole("button", { name: /^sshc dev/ })).toHaveCount(0);
});

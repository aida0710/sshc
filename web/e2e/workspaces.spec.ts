import { expect, test, openApplication } from "./support/environment";

test("reveals workspace counts together with actions on hover and keyboard focus", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const main = navigation.getByRole("button", { name: /^Main/ });
  const count = main.getByText("0", { exact: true });
  const newSession = navigation.getByRole("button", { name: "Open a session in Main" });
  await expect(count).toHaveCSS("opacity", "0");
  await expect(newSession).toHaveCSS("opacity", "0");
  await main.hover();
  await expect(count).toHaveCSS("opacity", "1");
  await expect(newSession).toHaveCSS("opacity", "1");
  await page.mouse.move(0, 0);
  await expect(count).toHaveCSS("opacity", "0");
  await main.focus();
  await expect(count).toHaveCSS("opacity", "1");
  await page.keyboard.press("Tab");
  await expect(newSession).toBeFocused();
  await expect(count).toHaveCSS("opacity", "1");
});

test.describe("touch sidebar", () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 800 } });

  test("keeps workspace counts and actions visible without shrinking tap areas", async ({ page, installation }) => {
    await page.route("**/api/v1/terminal/sessions", (route) => route.fulfill({ json: {
      sessions: [{ id: "shell", kind: "shell", workspaceId: "default", title: "Local shell", state: "connected", problem: "", startedAt: "2026-10-23T00:00:00Z" }],
      maxSessions: 50,
    } }));
    await openApplication(page, installation);
    await page.getByRole("button", { name: "Navigation", exact: true }).click();
    const navigation = page.getByRole("navigation", { name: "Primary" });
    const main = navigation.getByRole("button", { name: /^Main/ });
    await expect(main.getByText("1", { exact: true })).toHaveCSS("opacity", "1");
    await expect(navigation.getByRole("button", { name: "Open a session in Main" })).toHaveCSS("opacity", "1");
    expect((await main.boundingBox())?.height).toBeGreaterThanOrEqual(40);
    const session = navigation.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
    expect((await session.boundingBox())?.height).toBeGreaterThanOrEqual(40);
  });
});

test("groups sessions into workspaces and keeps a hidden one out of the tree", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const tree = navigation.getByRole("list", { name: "Workspaces" });
  await expect(tree.getByRole("button", { name: /^Main/ })).toBeVisible();
  await expect(tree.getByRole("button", { name: /^Agents/ })).toHaveCount(0);

  await navigation.getByRole("button", { name: "New workspace" }).click();
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

test("moves a session to another workspace by dragging it onto the workspace", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const tree = navigation.getByRole("list", { name: "Workspaces" });
  await navigation.getByRole("button", { name: "New workspace" }).click();
  await page.getByRole("textbox", { name: "Workspace name" }).fill("sshc dev");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const main = tree.getByRole("button", { name: /^Main/ });
  const workspace = tree.getByRole("button", { name: /^sshc dev/ });
  await expect(workspace).toBeVisible();

  await main.hover();
  await navigation.getByRole("button", { name: "Open a session in Main" }).click();
  const dialog = page.getByRole("dialog", { name: "New session" });
  await dialog.getByRole("button", { name: /^Local shell, localhost/ }).click();
  await expect(dialog).toBeHidden();
  await expect(main).toContainText("1");

  const session = tree.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first();
  await session.dragTo(workspace);
  await expect(workspace).toContainText("1");
  await expect(main).toContainText("0");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await navigation.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/workspace-after-drag.png` });
  }
});

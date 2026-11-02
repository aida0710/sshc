import { expect, test, openApplication, openLocalShell, sessionStatus } from "./support/environment";
import { drawnRows, terminalKeyboard } from "./support/terminal";

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

test("keeps the Workspaces heading at the top of the sidebar while the workspaces scroll", async ({ page, installation }) => {
  const workspaces = [
    { id: "default", name: "", hidden: false, builtIn: true },
    ...Array.from({ length: 14 }, (_, index) => ({ id: `w${index}`, name: `Workspace ${index + 1}`, hidden: false, builtIn: false })),
  ];
  await page.route(/\/api\/v1\/terminal\/workspaces$/, (route) =>
    route.request().method() === "GET" ? route.fulfill({ json: { workspaces, maxWorkspaces: 50 } }) : route.fallback());
  await page.setViewportSize({ width: 1280, height: 520 });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByRole("button", { name: /^Workspace 14/ })).toBeAttached();
  const scroller = page.locator("[data-navigation-scroll]");
  const heading = navigation.getByRole("button", { name: "New workspace" });

  await scroller.evaluate((element) => { element.scrollTop = element.scrollHeight; });

  const scrollerBox = await scroller.boundingBox();
  const headingBox = await heading.boundingBox();
  expect(scrollerBox).not.toBeNull();
  expect(headingBox).not.toBeNull();
  expect(headingBox!.y).toBeGreaterThanOrEqual(scrollerBox!.y);
  expect(headingBox!.y).toBeLessThan(scrollerBox!.y + 30);
  await expect(heading).toBeInViewport();
});

test.describe("touch sidebar", () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 800 } });

  test("keeps workspace counts and actions visible, with 36px rows and buttons", async ({ page, installation }) => {
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
    expect((await main.boundingBox())?.height).toBe(36);
    const session = navigation.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
    expect((await session.boundingBox())?.height).toBe(36);
    for (const name of ["Open a session in Main", "Workspace actions for Main", "Actions for Local shell", "Close Local shell"]) {
      const box = await navigation.getByRole("button", { name, exact: true }).boundingBox();
      expect(box?.width, name).toBe(36);
      expect(box?.height, name).toBe(36);
    }
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

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    // Capture the current workspace tree and run-target control before hiding the session.
    const terminal = page.getByRole("region", { name: /^Terminal for / });
    await expect(terminal).toContainText(/[$#%>]/);
    // Keep the capture independent of the machine's user name and hostname.
    await terminalKeyboard(page).focus();
    await page.keyboard.type("export PS1='sshc@localhost:~$ '; printf '\\033[2J\\033[H'");
    await page.keyboard.press("Enter");
    await expect(drawnRows(page)).toHaveText("sshc@localhost:~$");
    await page.mouse.move(0, 0);
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-desktop.png`, animations: "disabled" });
    await navigation.getByRole("button", { name: /^Choose where terminals run/ }).click();
    await expect(navigation.getByRole("list", { name: "Engines" })).toBeVisible();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/run-target-desktop.png`, animations: "disabled" });
    await page.keyboard.press("Escape");
  }

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

test("reorders workspaces by dragging a row and from a workspace's menu, and the engine keeps the order", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const tree = navigation.getByRole("list", { name: "Workspaces" });
  // The name on the first button of each workspace, top to bottom.
  const workspaceOrder = () => tree.evaluate((list) =>
    [...list.children].map((item) => item.querySelector("button")?.textContent?.replace(/\d+$/, "") ?? ""));
  await navigation.getByRole("button", { name: "New workspace" }).click();
  await page.getByRole("textbox", { name: "Workspace name" }).fill("sshc dev");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const main = tree.getByRole("button", { name: /^Main/ });
  const workspace = tree.getByRole("button", { name: /^sshc dev/ });
  await expect(workspace).toBeVisible();
  await expect.poll(workspaceOrder).toEqual(["Main", "sshc dev"]);

  // Held over the upper half of Main, the line shows where it goes; let go, it goes before Main.
  const from = (await workspace.boundingBox())!;
  const to = (await main.locator("xpath=ancestor::li[1]").boundingBox())!;
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + 4, { steps: 8 });
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await navigation.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/workspace-reorder-drag.png` });
  }
  await page.mouse.up();
  await expect.poll(workspaceOrder).toEqual(["sshc dev", "Main"]);

  await page.reload();
  await expect.poll(workspaceOrder).toEqual(["sshc dev", "Main"]);

  await workspace.hover();
  await navigation.getByRole("button", { name: "Workspace actions for sshc dev" }).click();
  await expect(page.getByRole("menuitem", { name: "Move up" })).toBeDisabled();
  await page.getByRole("menuitem", { name: "Move down" }).click();
  await expect.poll(workspaceOrder).toEqual(["Main", "sshc dev"]);
});

// Like the group headings of the connection list, a workspace's name stays under the Workspaces heading while
// its sessions scroll by.
test("keeps a workspace's name under the Workspaces heading while its sessions scroll", async ({ page, installation }) => {
  const workspaces = [
    { id: "default", name: "", hidden: false, builtIn: true },
    { id: "w-gpu", name: "GPU", hidden: false, builtIn: false },
  ];
  const sessions = Array.from({ length: 16 }, (_, index) => ({
    id: `s${index}`, kind: "shell", workspaceId: index < 8 ? "default" : "w-gpu", title: `Session ${index + 1}`,
    startedAt: "2026-10-28T00:00:00Z", state: "connected", problem: "", forwards: [],
  }));
  await page.route("**/api/v1/terminal/sessions**", (route) => new URL(route.request().url()).pathname.endsWith("/stream")
    ? route.fulfill({ status: 404, json: {} })
    : route.fulfill({ json: { sessions, maxSessions: 50 } }));
  await page.route("**/api/v1/terminal/workspaces", (route) => route.fulfill({ json: { workspaces, maxWorkspaces: 16 } }));
  await page.setViewportSize({ width: 1280, height: 640 });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByText("Session 16")).toBeAttached({ timeout: 20_000 });

  const list = page.locator("[data-navigation-scroll]");
  const heading = navigation.getByText("Workspaces", { exact: true });
  const main = navigation.getByRole("button", { name: /^Main/ });
  await list.evaluate((element) => { element.scrollTop = 120; });
  const headingBox = (await heading.boundingBox())!;
  const listBox = (await list.boundingBox())!;
  const mainBox = (await main.boundingBox())!;
  expect(Math.abs(headingBox.y - listBox.y)).toBeLessThan(8);
  expect(mainBox.y).toBeGreaterThanOrEqual(headingBox.y + headingBox.height - 2);
  expect(mainBox.y).toBeLessThan(headingBox.y + headingBox.height + 16);
});

// On a PC-sized screen a session's name has the whole row: its 「…」 and close show only under the pointer (the
// selected row too), over the end of the name, and a right click opens the same menu.
test("shows a session's actions only on hover and opens its menu with a right click", async ({ page, installation }) => {
  const sessions = [
    { id: "s1", kind: "shell", workspaceId: "default", title: "Build", startedAt: "2026-10-28T00:00:00Z", state: "connected", problem: "", forwards: [] },
    { id: "s2", kind: "shell", workspaceId: "default", title: "Logs", startedAt: "2026-10-28T00:00:00Z", state: "connected", problem: "", forwards: [] },
  ];
  await page.route("**/api/v1/terminal/sessions**", (route) => new URL(route.request().url()).pathname.endsWith("/stream")
    ? route.fulfill({ status: 404, json: {} })
    : route.fulfill({ json: { sessions, maxSessions: 50 } }));
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const actions = navigation.getByRole("button", { name: "Actions for Build" }).locator("..");
  await expect(actions).toHaveCSS("opacity", "0");
  await expect(actions).toHaveCSS("position", "absolute");

  await navigation.getByText("Build", { exact: true }).hover();
  await expect(actions).toHaveCSS("opacity", "1");

  await page.mouse.move(0, 0);
  await navigation.getByText("Logs", { exact: true }).click({ button: "right" });
  await expect(navigation.getByRole("button", { name: "Actions for Logs" })).toHaveAttribute("aria-expanded", "true");
});

// A PC with a touch screen can say it has no pointer that hovers (any-hover: none) while a mouse is in use. The
// actions hidden at PC size must still come up under the mouse there, not only while a press holds the focus.
test.describe("touch PC", () => {
  test.use({ viewport: { width: 1280, height: 800 }, hasTouch: true });

  test("shows a session's actions under the mouse though the screen says nothing hovers", async ({ page, installation }) => {
    const sessions = [
      { id: "s1", kind: "shell", workspaceId: "default", title: "Build", startedAt: "2026-10-28T00:00:00Z", state: "connected", problem: "", forwards: [] },
    ];
    await page.route("**/api/v1/terminal/sessions**", (route) => new URL(route.request().url()).pathname.endsWith("/stream")
      ? route.fulfill({ status: 404, json: {} })
      : route.fulfill({ json: { sessions, maxSessions: 50 } }));
    await openApplication(page, installation);
    expect(await page.evaluate(() => matchMedia("(any-hover: hover)").matches)).toBe(false);
    const navigation = page.getByRole("navigation", { name: "Primary" });
    const actions = navigation.getByRole("button", { name: "Actions for Build" }).locator("..");
    await expect(actions).toHaveCSS("opacity", "0");

    await navigation.getByText("Build", { exact: true }).hover();
    await expect(actions).toHaveCSS("opacity", "1");
    await expect(navigation.getByRole("button", { name: "Close Build" })).toBeVisible();
  });
});

test("shows the sessions and workspaces another browser makes, even while this one has no session", async ({ page, installation, browser, context }) => {
  await openApplication(page, installation);
  const second = await browser.newContext({ storageState: await context.storageState() });
  try {
    const other = await second.newPage();
    await other.goto(page.url());
    await expect(sessionStatus(other)).toContainText("Local session active");
    const otherTree = other.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Workspaces" });
    await expect(otherTree.getByRole("button", { name: /^Main/ })).toContainText("0");

    await openLocalShell(page);
    await expect(otherTree.getByRole("button", { name: /^Main/ })).toContainText("1");

    const navigation = page.getByRole("navigation", { name: "Primary" });
    await navigation.getByRole("button", { name: "New workspace" }).click();
    await page.getByRole("textbox", { name: "Workspace name" }).fill("made elsewhere");
    await page.getByRole("button", { name: "Create", exact: true }).click();
    await expect(otherTree.getByRole("button", { name: /^made elsewhere/ })).toBeVisible();
  } finally {
    await second.close();
  }
});

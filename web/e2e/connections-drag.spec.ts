import { mkdir, readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, test, type Installation } from "./support/environment";
import { groupList } from "./support/groups";
import { writeWorstCaseHome } from "./support/worstCaseData";

// From this width Connections has its groups in a column beside the card (xl in ConnectionGroupFacets). Below it they
// float while something is dragged.
const groupColumnWidth = 1280;
const windowHeight = 720;
const windowWidths = [1024, 1100, 1279, groupColumnWidth] as const;
// The editor beside the list at 1024px. With the groups as a column it had about 170px, too few to read a connection's name.
const narrowestReadableEditorWidth = 400;
// Far enough from where the mouse went down for the browser to begin a drag.
const dragBeginDistance = 12;
const mouseSteps = 4;
const hiddenNavigationKey = "sshc.navigation.hidden.v1";
const moveRequests = /\/api\/v1\/config\/(save|groups\/rename)$/;
const visualDirectory = process.env.SSHC_VISUAL_DIR;

// work holds web, archive holds nothing yet, and alpha and beta start in the entry file.
async function installGroups(installation: Installation): Promise<void> {
  const entry = await installation.read("config");
  await installation.write(
    "config",
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.\n" +
      "# Edit through the UI; lines between these markers are replaced on the next save.\n" +
      "Include connections/work/*.conf\n" +
      "Include connections/archive/*.conf\n" +
      "Include groups.sshc.conf\n" +
      "# <<< sshc groups\n" +
      entry +
      "\nHost alpha\n\tHostName 192.0.2.11\n\nHost beta\n\tHostName 192.0.2.12\n",
  );
  await installation.write("connections/work/web.conf", "Host web\n\tHostName 192.0.2.20\n");
  await mkdir(join(installation.home, ".ssh", "connections", "archive"), { recursive: true });
}

// goToConnections follows the address rather than the navigation's link, which a hidden navigation and a phone do not show.
async function goToConnections(page: Page): Promise<Locator> {
  await page.evaluate(() => {
    window.history.pushState(null, "", "/connections");
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  const tree = page.getByRole("navigation", { name: "Connections" });
  await expect(tree).toBeVisible();
  return tree;
}

async function openConnectionsAt(page: Page, installation: Installation, width: number): Promise<Locator> {
  await page.setViewportSize({ width, height: windowHeight });
  await openApplication(page, installation);
  return goToConnections(page);
}

async function hideNavigation(page: Page): Promise<void> {
  await page.addInitScript((key) => window.localStorage.setItem(key, "hidden"), hiddenNavigationKey);
}

async function centerOf(element: Locator): Promise<{ x: number; y: number }> {
  const box = await element.boundingBox();
  if (box === null) throw new Error("the element has no box on the screen");
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

// beginDrag presses on source and moves just far enough to begin the drag, as a person does. locator.dragTo cannot be
// used: it waits for its target to be visible before moving the mouse, and below the column's width the target only
// appears once the drag has begun.
async function beginDrag(page: Page, source: Locator): Promise<void> {
  const { x, y } = await centerOf(source);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + dragBeginDistance, y + dragBeginDistance, { steps: mouseSteps });
}

// overGroup moves the dragged thing over a group in the list of groups, and sees the group answer that it can land there.
async function overGroup(page: Page, name: string, groups = groupList(page)): Promise<void> {
  const group = groups.getByRole("button", { name, exact: true });
  const { x, y } = await centerOf(group);
  await page.mouse.move(x, y, { steps: mouseSteps });
  // The first move onto an element only enters it. The browser sends dragover from the next one, and a person's hand
  // never stops sending them.
  await page.mouse.move(x + 1, y);
  await expect(group.locator("xpath=parent::*")).toHaveClass(/outline-solid/);
}

// releaseAndAwaitMove lets go and waits until the engine has answered the move.
async function releaseAndAwaitMove(page: Page): Promise<void> {
  const [response] = await Promise.all([
    page.waitForResponse((candidate) => moveRequests.test(new URL(candidate.url()).pathname) && candidate.request().method() === "POST"),
    page.mouse.up(),
  ]);
  expect(response.status()).toBe(200);
}

async function expectInWorkGroup(installation: Installation, alias: string): Promise<void> {
  const folder = join(installation.home, ".ssh", "connections", "work");
  const files = await readdir(folder);
  const contents = await Promise.all(files.map((file) => readFile(join(folder, file), "utf8")));
  expect(contents.join("\n")).toContain(`Host ${alias}\n`);
  expect(await installation.read("config")).not.toContain(`Host ${alias}\n`);
}

// A group's heading in the connection list: it can be dragged, as the list of groups cannot be seen at every width.
function groupHeading(tree: Locator, name: string, connectionCount: number): Locator {
  return tree.getByRole("region", { name: `${name} group, ${connectionCount} connections` }).locator("header");
}

for (const width of windowWidths) {
  const groupsFloat = width < groupColumnWidth;

  test(`moves a connection onto a group with the editor open, in a window ${width}px wide`, async ({ page, installation }) => {
    await installGroups(installation);
    const tree = await openConnectionsAt(page, installation, width);
    await tree.getByRole("button", { name: "beta", exact: true }).click();
    const editor = page.getByRole("tablist", { name: "Connection editor" });
    await expect(editor).toBeVisible();
    await expect(groupList(page)).toBeVisible({ visible: !groupsFloat });
    const editorBefore = await editor.boundingBox();
    const treeBefore = await tree.boundingBox();
    expect(editorBefore?.width).toBeGreaterThan(narrowestReadableEditorWidth);

    await beginDrag(page, tree.getByRole("button", { name: "alpha", exact: true }));
    await expect(groupList(page)).toBeVisible();
    await overGroup(page, "work");

    // The list stays out of the page: the connection list and the editor keep their places and their widths.
    expect(await editor.boundingBox()).toEqual(editorBefore);
    expect(await tree.boundingBox()).toEqual(treeBefore);
    if (groupsFloat) {
      expect(await groupList(page).boundingBox()).toEqual(await page.getByRole("navigation", { name: "Primary" }).boundingBox());
    }

    await releaseAndAwaitMove(page);
    await expect(groupHeading(tree, "work", 2)).toBeVisible();
    await expect(tree.getByRole("region", { name: "work group, 2 connections" }).getByRole("button", { name: "alpha", exact: true })).toBeVisible();
    await expectInWorkGroup(installation, "alpha");
    await expect(groupList(page)).toBeVisible({ visible: !groupsFloat });
  });

  test(`moves a group by its heading onto another group, in a window ${width}px wide`, async ({ page, installation }) => {
    await installGroups(installation);
    const tree = await openConnectionsAt(page, installation, width);

    await beginDrag(page, groupHeading(tree, "work", 1));
    await expect(groupList(page)).toBeVisible();
    await overGroup(page, "archive");
    await releaseAndAwaitMove(page);

    await expect(groupHeading(tree, "archive/work", 1)).toBeVisible();
    expect(await installation.read("connections/archive/work/web.conf")).toContain("Host web\n");
    expect(await installation.read("config")).toContain("Include connections/archive/work/*.conf\n");
    await expect(groupList(page)).toBeVisible({ visible: !groupsFloat });
  });
}

test("still moves a group by its row in the column beside the card", async ({ page, installation }) => {
  await installGroups(installation);
  await openConnectionsAt(page, installation, groupColumnWidth);

  await beginDrag(page, groupList(page).getByRole("button", { name: "archive", exact: true }));
  await overGroup(page, "work");
  await releaseAndAwaitMove(page);

  expect(await installation.read("config")).toContain("Include connections/work/archive/*.conf\n");
});

for (const width of [1024, 1279] as const) {
  test(`floats the groups over the editor when the navigation is hidden, in a window ${width}px wide`, async ({ page, installation }) => {
    await installGroups(installation);
    await hideNavigation(page);
    const tree = await openConnectionsAt(page, installation, width);
    await expect(page.getByRole("navigation", { name: "Primary" })).toBeHidden();
    await tree.getByRole("button", { name: "beta", exact: true }).click();
    const editor = page.getByRole("tablist", { name: "Connection editor" });
    await expect(editor).toBeVisible();
    const editorBefore = await editor.boundingBox();

    await beginDrag(page, tree.getByRole("button", { name: "alpha", exact: true }));
    await expect(groupList(page)).toBeVisible();
    await overGroup(page, "work");

    // It covers no connection: the whole list lies to its left.
    const groups = await groupList(page).boundingBox();
    const treeBox = await tree.boundingBox();
    expect((treeBox?.x ?? Infinity) + (treeBox?.width ?? 0)).toBeLessThanOrEqual(groups?.x ?? 0);
    expect((groups?.x ?? Infinity) + (groups?.width ?? 0)).toBeLessThanOrEqual(width);
    expect(await editor.boundingBox()).toEqual(editorBefore);

    await releaseAndAwaitMove(page);
    await expectInWorkGroup(installation, "alpha");
    await expect(groupList(page)).toBeHidden();
  });

  test(`moves a group with the navigation hidden, in a window ${width}px wide`, async ({ page, installation }) => {
    await installGroups(installation);
    await hideNavigation(page);
    const tree = await openConnectionsAt(page, installation, width);

    await beginDrag(page, groupHeading(tree, "work", 1));
    await overGroup(page, "archive");
    await releaseAndAwaitMove(page);

    expect(await installation.read("config")).toContain("Include connections/archive/work/*.conf\n");
    await expect(groupList(page)).toBeHidden();
  });
}

test("does not float the groups under the pointer when a row of the full-width list is taken by its right end", async ({ page, installation }) => {
  await installGroups(installation);
  await hideNavigation(page);
  const tree = await openConnectionsAt(page, installation, 1024);
  const row = tree.getByRole("button", { name: "alpha", exact: true });
  const box = await row.boundingBox();
  if (box === null) throw new Error("the row has no box on the screen");

  const grabbedX = box.x + box.width - 4;
  await page.mouse.move(grabbedX, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(grabbedX - 2, box.y + box.height / 2 + dragBeginDistance, { steps: mouseSteps });
  await expect(groupList(page)).toBeVisible();

  const groups = await groupList(page).boundingBox();
  expect((groups?.x ?? Infinity) + (groups?.width ?? 0)).toBeLessThan(grabbedX);
  await overGroup(page, "work");
  await releaseAndAwaitMove(page);
  await expectInWorkGroup(installation, "alpha");
});

test("moves every chosen connection when one of them is dragged", async ({ page, installation }) => {
  await installGroups(installation);
  const tree = await openConnectionsAt(page, installation, 1024);
  await tree.getByRole("button", { name: "alpha", exact: true }).click();
  await tree.getByRole("button", { name: "beta", exact: true }).click({ modifiers: ["Control"] });
  await expect(page.getByRole("toolbar", { name: "2 selected" })).toBeVisible();

  await beginDrag(page, tree.getByRole("button", { name: "beta", exact: true }));
  await overGroup(page, "work");
  await releaseAndAwaitMove(page);

  await expectInWorkGroup(installation, "alpha");
  await expectInWorkGroup(installation, "beta");
});

test("moves nothing and puts the list away when the drag ends outside the groups", async ({ page, installation }) => {
  await installGroups(installation);
  const tree = await openConnectionsAt(page, installation, 1024);
  const saves: string[] = [];
  page.on("request", (request) => {
    if (moveRequests.test(new URL(request.url()).pathname)) saves.push(request.url());
  });

  await beginDrag(page, tree.getByRole("button", { name: "alpha", exact: true }));
  await expect(groupList(page)).toBeVisible();
  await page.mouse.up();

  await expect(groupList(page)).toBeHidden();
  expect(saves).toEqual([]);
  expect(await installation.read("config")).toContain("Host alpha\n");
});

test("moves a group with no connections, which has no heading in the list, inside the group sheet", async ({ page, installation }) => {
  await installGroups(installation);
  const tree = await openConnectionsAt(page, installation, 1024);
  await expect(tree.getByRole("region", { name: /^archive group/ })).toHaveCount(0);
  await page.getByRole("main").getByRole("button", { name: "Group: All" }).click();
  const sheet = page.getByRole("dialog", { name: "Groups" });

  await beginDrag(page, sheet.getByRole("button", { name: /^archive/ }));
  const { x, y } = await centerOf(sheet.getByRole("button", { name: /^work/ }));
  await page.mouse.move(x, y, { steps: mouseSteps });
  // The sheet holds every group, so the list does not float behind it.
  await expect(groupList(page)).toBeHidden();
  await releaseAndAwaitMove(page);

  expect(await installation.read("config")).toContain("Include connections/work/archive/*.conf\n");
  await expect(sheet.getByRole("button", { name: /^archive/ })).toBeVisible();
});

test("offers no drag of a group on a phone, and no list of groups", async ({ page, installation }) => {
  await installGroups(installation);
  const tree = await openConnectionsAt(page, installation, 412);

  await expect(tree.locator("header[draggable=true]")).toHaveCount(0);
  await expect(groupList(page)).toHaveCount(0);
  await page.getByRole("main").getByRole("button", { name: "Group: All" }).click();
  const sheet = page.getByRole("dialog", { name: "Groups" });
  await expect(sheet.getByRole("button", { name: /^work/ })).toBeVisible();
  await expect(sheet.locator("button[draggable=true]")).toHaveCount(0);
});

test("keeps a long list of groups inside a short window and scrolls it while dragging", async ({ page, installation }) => {
  const manyGroups = Array.from({ length: 40 }, (_, index) => `team-${String(index).padStart(2, "0")}`);
  const entry = await installation.read("config");
  await installation.write(
    "config",
    "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.\n" +
      "# Edit through the UI; lines between these markers are replaced on the next save.\n" +
      `${manyGroups.map((group) => `Include connections/${group}/*.conf`).join("\n")}\n` +
      "Include groups.sshc.conf\n" +
      "# <<< sshc groups\n" +
      entry +
      "\nHost alpha\n\tHostName 192.0.2.11\n",
  );
  for (const group of manyGroups) await mkdir(join(installation.home, ".ssh", "connections", group), { recursive: true });
  const tree = await openConnectionsAt(page, installation, 1024);

  await beginDrag(page, tree.getByRole("button", { name: "alpha", exact: true }));
  await expect(groupList(page)).toBeVisible();
  const groups = await groupList(page).boundingBox();
  expect((groups?.y ?? Infinity) + (groups?.height ?? 0)).toBeLessThanOrEqual(windowHeight);
  const rows = groupList(page).locator("[data-connection-facets]");
  const scrollTop = () => rows.evaluate((element) => element.scrollTop);
  expect(await rows.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);

  // Held near the bottom of the list, the browser scrolls it.
  const rowsBox = await rows.boundingBox();
  if (rowsBox === null) throw new Error("the rows have no box on the screen");
  const nearBottom = { x: rowsBox.x + rowsBox.width / 2, y: rowsBox.y + rowsBox.height - 4 };
  await expect.poll(async () => {
    await page.mouse.move(nearBottom.x, nearBottom.y);
    return scrollTop();
  }).toBeGreaterThan(0);

  await page.mouse.up();
  await expect(groupList(page)).toBeHidden();
});

// The pictures the change was read by, over the worst data and in Japanese. Only with SSHC_VISUAL_DIR.
const pictureCases = [
  { name: "editor-open", width: 1024, navigationHidden: false, editorOpen: true },
  { name: "nothing-open", width: 1024, navigationHidden: false, editorOpen: false },
  { name: "wide-window", width: 1279, navigationHidden: false, editorOpen: true },
  { name: "navigation-hidden-editor-open", width: 1024, navigationHidden: true, editorOpen: true },
  { name: "navigation-hidden-nothing-open", width: 1024, navigationHidden: true, editorOpen: false },
  { name: "column", width: groupColumnWidth, navigationHidden: false, editorOpen: true },
] as const;

for (const pictureCase of pictureCases) {
  test(`takes a picture of the drag: ${pictureCase.name}`, async ({ page, installation }) => {
    test.skip(visualDirectory === undefined, "only with SSHC_VISUAL_DIR");
    await writeWorstCaseHome(installation);
    if (pictureCase.navigationHidden) await hideNavigation(page);
    await openApplication(page, installation);
    await changeDisplayLanguage(page, "ja");
    await page.setViewportSize({ width: pictureCase.width, height: windowHeight });
    const tree = await goToConnections(page);
    const source = tree.getByRole("button", { name: "frankfurt-api-01", exact: true });
    if (pictureCase.editorOpen) {
      await tree.getByRole("button", { name: "frankfurt-api-02", exact: true }).click();
      await expect(page.locator("[data-connection-summary]")).toBeVisible();
    }

    await beginDrag(page, source);
    const groups = page.getByRole("navigation", { name: "グループ" });
    await expect(groups).toBeVisible();
    await overGroup(page, "fleet-tokyo", groups);
    await page.screenshot({ path: join(visualDirectory ?? "", `drag-${pictureCase.name}-${pictureCase.width}.png`) });
    await page.mouse.up();
  });
}

import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openLocalShell, test } from "./support/environment";
import { terminalHostBottomOverflow, terminalKeyboard, waitForTerminalLayout } from "./support/terminal";

function pane(page: Page, sessionId: string): Locator {
  return page.locator(`[data-split-view-pane="${sessionId}"]`);
}

async function openNamedShell(page: Page, title: string): Promise<string> {
  const previousIds = await page.locator("[data-split-view-pane]").evaluateAll((elements) => elements.map((element) => element.getAttribute("data-split-view-pane")));
  await openLocalShell(page);
  const terminal = page.locator("[data-split-view-pane]");
  await expect(terminal).toHaveCount(1);
  await expect.poll(async () => previousIds.includes(await terminal.getAttribute("data-split-view-pane"))).toBe(false);
  await expect(terminal.getByRole("region")).toContainText(/[$#%>]/);
  const sessionId = await terminal.getAttribute("data-split-view-pane");
  if (sessionId === null) throw new Error("missing shell session ID");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const row = navigation.getByRole("list", { name: "Open sessions" }).getByRole("listitem").filter({ has: page.locator('[aria-current="true"]') });
  await row.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
  const input = navigation.getByRole("textbox", { name: /^New name for / });
  await input.fill(title);
  await input.press("Enter");
  await expect(navigation.getByRole("button", { name: title, exact: true })).toBeVisible();
  return sessionId;
}

async function splitWithMenu(page: Page, title: string, edge: "right" | "below" = "right") {
  await page.getByRole("button", { name: `Actions for ${title}`, exact: true }).click();
  const action = page.getByRole("menuitem", { name: edge === "right" ? "Show in split view on the right" : "Show in split view below", exact: true });
  await action.focus();
  await action.press("Enter");
}

async function runCommand(terminal: Locator, command: string) {
  const keyboard = terminalKeyboard(terminal);
  await keyboard.focus();
  await keyboard.pressSequentially(command);
  await keyboard.press("Enter");
}

async function dragSession(page: Page, options: { title: string; target: Locator; edge: "left" | "right" | "top" | "bottom" }) {
  const row = page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" }).getByRole("listitem").filter({ has: page.getByRole("button", { name: options.title, exact: true }) });
  const bounds = await options.target.boundingBox();
  if (bounds === null) throw new Error("missing pane bounds");
  const edgeInset = 8; // Stay within the pane's drop area, away from the divider.
  await row.dragTo(options.target, { targetPosition: {
    x: options.edge === "left" ? edgeInset : options.edge === "right" ? bounds.width - edgeInset : bounds.width / 2,
    y: options.edge === "top" ? edgeInset : options.edge === "bottom" ? bounds.height - edgeInset : bounds.height / 2,
  } });
}

test("splits real shells, routes input independently, resizes and closes only the pane", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Build");
  const two = await openNamedShell(page, "Logs");
  await page.getByRole("button", { name: "Build", exact: true }).click();
  await dragSession(page, { title: "Logs", target: pane(page, one), edge: "right" });
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  const left = pane(page, one);
  const right = pane(page, two);
  await expect(left).toBeVisible();
  await expect(right).toBeVisible();
  await expect(left.getByText("Build", { exact: true })).toHaveCount(1);
  await expect(right.getByText("Logs", { exact: true })).toHaveCount(1);
  for (const terminal of [left, right]) {
    const searchBounds = (await terminal.getByRole("button", { name: "Find", exact: true }).boundingBox())!;
    for (const action of await terminal.locator("[data-pane-toolbar] button").all()) {
      const actionBounds = (await action.boundingBox())!;
      expect(actionBounds.y).toBe(searchBounds.y);
    }
  }
  expect((await right.boundingBox())!.x).toBeGreaterThan((await left.boundingBox())!.x);
  await runCommand(left, "printf 'LEFT:%s\\n' $((123 + 456))");
  await expect(left).toContainText("LEFT:579");
  await expect(right).not.toContainText("LEFT:579");
  await runCommand(right, "printf 'RIGHT:%s\\n' $((789 + 321))");
  await expect(right).toContainText("RIGHT:1110");
  await expect(left).not.toContainText("RIGHT:1110");

  const separator = page.getByRole("separator", { name: "Resize split view" });
  const initialWidth = (await left.boundingBox())!.width;
  const box = (await separator.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + initialWidth / 3, box.y + box.height / 2);
  await page.mouse.up();
  await expect.poll(async () => Number(await separator.getAttribute("aria-valuenow"))).toBeGreaterThan(60);
  await expect.poll(async () => (await left.boundingBox())!.width).toBeGreaterThan(initialWidth);
  await waitForTerminalLayout(page);
  expect(await terminalHostBottomOverflow(left)).toBeLessThanOrEqual(1);
  expect(await terminalHostBottomOverflow(right)).toBeLessThanOrEqual(1);

  await right.getByRole("button", { name: "Close Logs pane" }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
  await expect(left).toBeVisible();
  await expect(left.locator("[data-pane-toolbar]")).toHaveCount(0);
  await page.getByRole("button", { name: "Logs", exact: true }).click();
  await expect(right).toContainText("RIGHT:1110");
  await runCommand(right, "printf 'STILL:%s\\n' $((6 * 7))");
  await expect(right).toContainText("STILL:42");
});

test("preserves terminal mounts through swaps, focus mode, sidebar selection and mobile widths", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Build");
  const two = await openNamedShell(page, "Logs");
  await splitWithMenu(page, "Build");
  const first = pane(page, one);
  const second = pane(page, two);
  await expect(first.getByRole("region")).toContainText(/[$#%>]/);
  // Opening search proves component state survives changing the pane tree.
  await terminalKeyboard(first).focus();
  await page.keyboard.press("Control+f");
  const search = first.getByRole("textbox", { name: "Search terminal output" });
  await search.fill("mount-canary");
  await first.getByRole("button", { name: "Swap Build pane" }).click();
  await second.getByRole("button", { name: "Swap Logs pane" }).click();
  expect((await first.boundingBox())!.x).toBeLessThan((await second.boundingBox())!.x);
  await expect(search).toHaveValue("mount-canary");
  await first.getByRole("button", { name: "Focus Build pane" }).click();
  await expect(second).toBeHidden();
  await page.getByRole("button", { name: "Logs", exact: true }).click();
  await expect(second).toBeVisible();
  await expect(first).toBeHidden();
  await terminalKeyboard(second).focus();
  await page.keyboard.press("Escape");
  await expect(first).toBeVisible();
  await page.keyboard.press("Control+Alt+ArrowLeft");
  await expect(first).toHaveAttribute("data-pane-active", "true");
  await expect(terminalKeyboard(first)).toBeFocused();

  await page.setViewportSize({ width: 390, height: 800 });
  await expect(page.getByRole("separator", { name: "Resize split view" })).toHaveCount(0);
  await expect(page.locator("[data-split-view-pane]:visible")).toHaveCount(1);
  await expect(page.locator("[data-pane-toolbar]")).toHaveCount(0);
  const switcher = page.getByRole("navigation", { name: "Split view panes" });
  await switcher.getByRole("button", { name: "Logs", exact: true }).click();
  await expect(second).toBeVisible();
  await expect(first).toBeHidden();
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect(first).toBeVisible();
  await expect(search).toHaveValue("mount-canary");
  await first.getByRole("button", { name: "Swap Build pane" }).dragTo(second);
  expect((await first.boundingBox())!.x).toBeGreaterThan((await second.boundingBox())!.x);
});

test("splits across workspaces without changing membership and still accepts workspace drops", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Build");
  const two = await openNamedShell(page, "Logs");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New workspace", exact: true }).click();
  await page.getByRole("textbox", { name: "Workspace name" }).fill("Monitoring");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const workspace = navigation.getByRole("button", { name: /^Monitoring/ });
  const row = navigation.getByRole("list", { name: "Open sessions" }).getByRole("listitem").filter({ has: page.getByRole("button", { name: "Logs", exact: true }) });
  await row.dragTo(workspace);
  await expect(workspace).toContainText("1");
  await splitWithMenu(page, "Build");
  await expect(pane(page, one)).toBeVisible();
  await expect(pane(page, two)).toBeVisible();
  await expect(workspace).toContainText("1");
  await row.dragTo(navigation.getByRole("button", { name: /^Main/ }));
  await expect(workspace).toContainText("0");
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
});

test("closes exited and deleted panes and clears the arrangement on reload", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Build");
  const two = await openNamedShell(page, "Logs");
  const three = await openNamedShell(page, "Checks");
  await page.getByRole("button", { name: "Build", exact: true }).click();
  await splitWithMenu(page, "Logs");
  await splitWithMenu(page, "Checks", "below");
  await runCommand(pane(page, three), "exit");
  await expect(pane(page, three)).toHaveCount(0);
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  await page.getByRole("button", { name: "Close Logs", exact: true }).click({ modifiers: ["Shift"] });
  await expect(pane(page, two)).toHaveCount(0);
  await expect(pane(page, one)).toBeVisible();
  await page.getByRole("button", { name: "Checks", exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
  await expect(pane(page, three)).toBeVisible();
  await openNamedShell(page, "New logs");
  await splitWithMenu(page, "Build");
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  await page.reload();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
});

test("limits split view to four panes and shows the Japanese desktop layout", async ({ page, installation }) => {
  // This scenario opens five real shells before exercising the four-pane limit.
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1280, height: 800 });
  await openApplication(page, installation);
  const titles = ["ビルド", "アプリのログ", "テスト", "マシンの状態", "予備"];
  const ids: string[] = [];
  for (const title of titles) ids.push(await openNamedShell(page, title));
  await page.getByRole("button", { name: titles[0]!, exact: true }).click();
  await splitWithMenu(page, titles[1]!);
  await dragSession(page, { title: titles[2]!, target: pane(page, ids[0]!), edge: "bottom" });
  await dragSession(page, { title: titles[3]!, target: pane(page, ids[1]!), edge: "bottom" });
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(4);
  await page.getByRole("button", { name: `Actions for ${titles[4]}`, exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Show in split view on the right" })).toBeDisabled();
  await page.keyboard.press("Escape");
  await dragSession(page, { title: titles[4]!, target: pane(page, ids[0]!), edge: "left" });
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(4);
  for (const [index, output] of ["build complete", "GET /health 200 OK", "tests passed", "all services running"].entries()) {
    await runCommand(pane(page, ids[index]!), `printf '\\033[2J\\033[H%s\\n' '${output}'`);
    await expect(pane(page, ids[index]!)).toContainText(output);
  }
  await changeDisplayLanguage(page, "ja");
  await expect(page.getByRole("navigation", { name: "メインナビゲーション" })).toBeVisible();
  await waitForTerminalLayout(page);
  // The existing 32px header plus the pane border must remain a single row.
  const maximumHeaderHeight = 34;
  for (const [index, id] of ids.slice(0, 4).entries()) {
    const terminal = pane(page, id);
    await expect(terminal.getByText(titles[index]!, { exact: true })).toHaveCount(1);
    expect(await terminalHostBottomOverflow(terminal)).toBeLessThanOrEqual(1);
    const paneBounds = (await terminal.boundingBox())!;
    const hostBounds = (await terminal.locator("[data-terminal-host]").boundingBox())!;
    expect(hostBounds.y - paneBounds.y).toBeLessThanOrEqual(maximumHeaderHeight);
  }
  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) await page.screenshot({ path: `${visualDirectory}/split-view-four-panes-ja.png` });
  for (const index of [2, 3]) {
    await pane(page, ids[index]!).getByRole("button", { name: `${titles[index]}のペインを閉じる`, exact: true }).click();
  }
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  const leftBounds = (await pane(page, ids[0]!).boundingBox())!;
  const rightBounds = (await pane(page, ids[1]!).boundingBox())!;
  expect(rightBounds.x).toBeGreaterThan(leftBounds.x);
  expect(rightBounds.y).toBe(leftBounds.y);
  await waitForTerminalLayout(page);
  if (visualDirectory !== undefined) await page.screenshot({ path: `${visualDirectory}/split-view-two-panes-ja.png` });
});

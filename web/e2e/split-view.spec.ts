import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRowsIn, terminalHostBottomOverflow, terminalKeyboard, waitForTerminalLayout } from "./support/terminal";

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

// focusedPane is the session whose pane holds the keyboard focus, or null when no pane does.
async function focusedPane(page: Page): Promise<string | null> {
  return page.evaluate(() => document.activeElement?.closest("[data-split-view-pane]")?.getAttribute("data-split-view-pane") ?? null);
}

function sessionRow(page: Page, title: string): Locator {
  return page.getByRole("navigation", { name: "Primary" }).getByRole("list", { name: "Open sessions" }).getByRole("listitem").filter({ has: page.getByRole("button", { name: title, exact: true }) });
}

async function dragSession(page: Page, options: { title: string; target: Locator; edge: "left" | "right" | "top" | "bottom" }) {
  const row = sessionRow(page, options.title);
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

test("keeps a pane of a nested split its header and a row of the terminal, however far it is dragged", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Top");
  const two = await openNamedShell(page, "Middle");
  await openNamedShell(page, "Bottom");
  await page.getByRole("button", { name: "Top", exact: true }).click();
  await dragSession(page, { title: "Middle", target: pane(page, one), edge: "bottom" });
  await dragSession(page, { title: "Bottom", target: pane(page, two), edge: "bottom" });
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(3);
  const middle = pane(page, two);

  // The second divider splits the lower half between Middle and Bottom. Drag it onto Middle's top.
  const inner = (await page.getByRole("separator", { name: "Resize split view" }).nth(1).boundingBox())!;
  const middleTop = (await middle.boundingBox())!.y;
  await page.mouse.move(inner.x + inner.width / 2, inner.y + inner.height / 2);
  await page.mouse.down();
  await page.mouse.move(inner.x + inner.width / 2, middleTop + 2, { steps: 4 });
  await page.mouse.up();

  await waitForTerminalLayout(page);
  expect((await middle.boundingBox())!.height).toBeGreaterThanOrEqual(79);
  await expect(drawnRowsIn(middle)).not.toHaveCount(0);
  // The pane shows one row, so the size the shell got is printed without a newline and held there.
  await runCommand(middle, "printf 'ROWS:%s' \"$(stty size)\"; sleep 30");
  await expect(middle).toContainText(/ROWS:[1-9]/);
});

test("keeps the header's buttons inside a pane of a nested split, however narrow it is dragged", async ({ page, installation }) => {
  await openApplication(page, installation);
  const one = await openNamedShell(page, "Left");
  const two = await openNamedShell(page, "Middle");
  await openNamedShell(page, "Right");
  await page.getByRole("button", { name: "Left", exact: true }).click();
  await dragSession(page, { title: "Middle", target: pane(page, one), edge: "right" });
  await dragSession(page, { title: "Right", target: pane(page, two), edge: "right" });
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(3);
  const middle = pane(page, two);

  const inner = (await page.getByRole("separator", { name: "Resize split view" }).nth(1).boundingBox())!;
  const middleLeft = (await middle.boundingBox())!.x;
  await page.mouse.move(inner.x + inner.width / 2, inner.y + inner.height / 2);
  await page.mouse.down();
  await page.mouse.move(middleLeft + 2, inner.y + inner.height / 2, { steps: 4 });
  await page.mouse.up();

  await waitForTerminalLayout(page);
  const bounds = (await middle.boundingBox())!;
  const close = (await middle.getByRole("button", { name: "Close Middle pane" }).boundingBox())!;
  expect(close.x + close.width).toBeLessThanOrEqual(bounds.x + bounds.width);
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

test("gives the keyboard to the pane whose header is clicked and frames that pane in the sidebar", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openApplication(page, installation);
  const build = await openNamedShell(page, "Build");
  const logs = await openNamedShell(page, "Logs");
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "Build", exact: true }).click();
  await pane(page, build).getByRole("button", { name: "More terminal actions", exact: true }).click();
  await page.getByRole("menuitem", { name: "Show in split view on the right", exact: true }).click();
  await page.getByRole("dialog", { name: "Choose a session to split" }).getByRole("button", { name: "Logs", exact: true }).click();
  await expect.poll(() => focusedPane(page)).toBe(logs);
  const buildMember = navigation.getByRole("button", { name: "Build in Split 1", exact: true });
  const logsMember = navigation.getByRole("button", { name: "Logs in Split 1", exact: true });
  await expect(logsMember).toHaveAttribute("aria-current", "true");
  await expect(buildMember).not.toHaveAttribute("aria-current", "true");

  // The title is plain text: before the fix the click chose the pane but left the keyboard on the page body.
  await pane(page, build).getByText("Build", { exact: true }).click();
  await expect.poll(() => focusedPane(page)).toBe(build);
  await page.keyboard.type("echo HEADER:$((6 * 7))");
  await page.keyboard.press("Enter");
  await expect(pane(page, build)).toContainText("HEADER:42");
  await expect(pane(page, logs)).not.toContainText("HEADER:42");
  await expect(buildMember).toHaveAttribute("aria-current", "true");
  await expect(logsMember).not.toHaveAttribute("aria-current", "true");

  await logsMember.click();
  await expect.poll(() => focusedPane(page)).toBe(logs);
  await expect(logsMember).toHaveAttribute("aria-current", "true");
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
  const row = sessionRow(page, "Logs");
  await row.dragTo(workspace);
  await expect(workspace).toContainText("1");
  await splitWithMenu(page, "Build");
  await expect(pane(page, one)).toBeVisible();
  await expect(pane(page, two)).toBeVisible();
  await expect(workspace).toContainText("1");
  await row.dragTo(navigation.getByRole("button", { name: /^Main/ }));
  await expect(workspace).toContainText("0");
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  await expect(row.locator('[data-split-group-badge="1"]')).toHaveCount(1);
});

test("keeps two split groups through standalone selection and dissolves them without closing sessions", async ({ page, installation }) => {
  // Five real shells provide two independent groups and one standalone session.
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1280, height: 800 });
  await openApplication(page, installation);
  const titles = ["ビルド", "アプリのログ", "テスト", "マシンの状態", "予備"];
  const outputs = [
    "Build completed successfully.\\nAssets are ready.",
    "14:32:01 INFO  Listening on localhost:3000\\n14:32:04 GET   /             200\\n14:32:07 GET   /health       200",
    "Test Files  3 passed (3)\\n     Tests  54 passed (54)\\n  Duration  1.28s",
    "All services running.",
    "Ready for the next command.",
  ];
  const ids: string[] = [];
  for (const [index, title] of titles.entries()) {
    const id = await openNamedShell(page, title);
    ids.push(id);
    // Playwright key events cannot type Japanese through xterm's composition path.
    // Wait for computed output so command echo cannot satisfy readiness.
    await runCommand(pane(page, id), `PS1='sshc@localhost:~/project$ '; printf '\\033[2J\\033[H${outputs[index]}\\nREADY:%s\\n' $((6 * 7))`);
    await expect(pane(page, id)).toContainText("READY:42");
  }
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "New workspace", exact: true }).click();
  await page.getByRole("textbox", { name: "Workspace name" }).fill("検証");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  const workspace = navigation.getByRole("button", { name: /^検証/ });
  for (const title of titles.slice(3)) await sessionRow(page, title).dragTo(workspace);
  await expect(workspace).toContainText("2");
  await navigation.getByRole("button", { name: titles[0]!, exact: true }).click();
  await pane(page, ids[0]!).getByRole("button", { name: "More terminal actions", exact: true }).click();
  await page.getByRole("menuitem", { name: "Show in split view on the right", exact: true }).click();
  await page.getByRole("dialog", { name: "Choose a session to split" }).getByRole("button", { name: titles[1]!, exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  await navigation.getByRole("button", { name: titles[3]!, exact: true }).click();
  await splitWithMenu(page, titles[4]!);
  await expect(page.locator("[data-split-group]")).toHaveCount(2);
  await expect(navigation.locator('[data-split-group-badge="1"]')).toHaveCount(2);
  await expect(navigation.locator('[data-split-group-badge="2"]')).toHaveCount(2);
  await navigation.getByRole("button", { name: titles[2]!, exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
  await expect(pane(page, ids[2]!)).toBeVisible();
  await navigation.getByRole("button", { name: "Split 1", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(pane(page, ids[0]!)).toBeVisible();
  await expect(pane(page, ids[1]!)).toBeVisible();
  await expect(terminalKeyboard(pane(page, ids[1]!))).toBeFocused();
  await navigation.getByRole("button", { name: titles[3]!, exact: true }).click();
  await expect(pane(page, ids[3]!)).toHaveAttribute("data-pane-active", "true");
  await expect(pane(page, ids[4]!)).toBeVisible();
  await navigation.getByRole("button", { name: "Split 1", exact: true }).click();
  await changeDisplayLanguage(page, "ja");
  const japaneseNavigation = page.getByRole("navigation", { name: "メインナビゲーション" });
  const groupOne = japaneseNavigation.locator('[data-split-group="1"]');
  // The group lists the sessions it holds under its row.
  await expect(groupOne.getByRole("list", { name: "分割 1のセッション" }).getByRole("button")).toHaveText([/ビルド/, /アプリのログ/]);
  await groupOne.getByRole("button", { name: "分割 1のアプリのログ", exact: true }).click();
  await expect(terminalKeyboard(pane(page, ids[1]!))).toBeFocused();
  await expect(japaneseNavigation.getByRole("list", { name: "Split views" })).toBeVisible();
  await waitForTerminalLayout(page);
  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) await page.screenshot({ path: `${visualDirectory}/split-groups-split-ja.png` });
  await japaneseNavigation.getByRole("button", { name: titles[2]!, exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
  await expect(pane(page, ids[2]!).getByRole("region")).toContainText("READY:42");
  await waitForTerminalLayout(page);
  if (visualDirectory !== undefined) await page.screenshot({ path: `${visualDirectory}/split-groups-single-ja.png` });
  await page.setViewportSize({ width: 390, height: 800 });
  await page.getByRole("button", { name: "ナビゲーション", exact: true }).click();
  await japaneseNavigation.getByRole("button", { name: "分割 2", exact: true }).click();
  await expect(page.locator("[data-split-view-pane]:visible")).toHaveCount(1);
  const mobilePanes = page.getByRole("navigation", { name: "分割表示のペイン" });
  await mobilePanes.getByRole("button", { name: titles[3]!, exact: true }).click();
  await expect(pane(page, ids[3]!)).toBeVisible();
  await expect(pane(page, ids[4]!)).toBeHidden();
  await page.setViewportSize({ width: 1280, height: 800 });
  await groupOne.getByRole("button", { name: "分割 1", exact: true }).click();
  await japaneseNavigation.getByRole("button", { name: "分割 2の操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "解除", exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(2);
  await expect(japaneseNavigation.locator('[data-split-group-badge="2"]')).toHaveCount(0);
  await japaneseNavigation.getByRole("button", { name: "分割 1の操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "解除", exact: true }).click();
  await expect(page.locator("[data-split-view-pane]")).toHaveCount(1);
  await expect(japaneseNavigation.getByRole("list", { name: "Split views" })).toHaveCount(0);
  await expect(japaneseNavigation.locator("[data-split-group-badge]")).toHaveCount(0);
  for (const title of titles) await expect(japaneseNavigation.getByRole("button", { name: title, exact: true })).toBeVisible();
  await runCommand(pane(page, ids[1]!), "printf 'SURVIVED:%s\\n' $((6 * 7))");
  await expect(pane(page, ids[1]!)).toContainText("SURVIVED:42");
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
  // The 40px header (the design's height) plus the board's border must remain a single row. The
  // board sits inside the pane's gap, so it is measured from the board, not the pane.
  const maximumHeaderHeight = 41;
  for (const [index, id] of ids.slice(0, 4).entries()) {
    const terminal = pane(page, id);
    await expect(terminal.getByText(titles[index]!, { exact: true })).toHaveCount(1);
    expect(await terminalHostBottomOverflow(terminal)).toBeLessThanOrEqual(1);
    const boardBounds = (await terminal.locator("[data-split-view-board]").boundingBox())!;
    const hostBounds = (await terminal.locator("[data-terminal-host]").boundingBox())!;
    expect(hostBounds.y - boardBounds.y).toBeLessThanOrEqual(maximumHeaderHeight);
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

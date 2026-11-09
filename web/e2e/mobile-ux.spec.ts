import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Locator, Page, WebSocketRoute } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test, type Installation } from "./support/environment";
import { samplePNG } from "./support/images";
import { chooseEntryOnPhone } from "./support/sftp";
import { drawnRowCount, drawnRows, drawnRowsIn, drawnSpan } from "./support/terminal";

// All screenshots are opt-in and contain only the isolated installation's
// example hosts and synthetic SFTP/terminal data. Never capture real servers.
async function capture(page: Page, name: string) {
  const directory = process.env.SSHC_MOBILE_VISUAL_DIR;
  if (directory === undefined) return;
  await mkdir(directory, { recursive: true });
  await page.screenshot({ path: join(directory, `${name}.png`), animations: "disabled" });
}

// Longer than the list waits before taking a press for a long one (useSFTPEntryList).
const longPressHeldMs = 700;

const entry = (name: string, type: "file" | "directory", parent = "/workspace") => ({
  name, path: `${parent}/${name}`, type, size: type === "file" ? 2048 : 0,
  modifiedAt: "2026-09-12T00:00:00Z", mode: type === "file" ? "0644" : "0755", revision: `${name}-v1`,
});
const rootEntries = [entry("projects", "directory"), entry("archives", "directory"), ...Array.from({ length: 12 }, (_, index) => entry(`report-${index + 1}.txt`, "file"))];
const transferList = {
  maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: true,
  largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, speedLimitBytesPerSecond: 0, autoReconnect: false, maxReconnectAttempts: 0,
  jobs: [{
    id: "mobile-demo-transfer", batchId: "mobile-demo-batch", batchName: "Demo download", batchKind: "file",
    alias: "bastion", sourceAlias: "", sourcePath: "", operation: "", direction: "download", kind: "file",
    name: "reports.zip", remotePath: "/workspace/reports.zip", totalBytes: 10 << 20, transferredBytes: 4 << 20,
    bytesPerSecond: 0, remainingSeconds: -1, status: "paused", allowedActions: ["resume", "cancel"],
    attempt: 1, reconnectAttempt: 0, reconnectAt: "", problem: "", lastModified: 0, expectedRevision: "", sourceFingerprint: "", overwrite: false,
    downloadRevision: "demo-revision", downloadParts: [], createdAt: "2026-09-12T00:00:00Z", updatedAt: "2026-09-12T00:00:00Z",
  }],
};

async function mockFiles(page: Page, waitForProjects?: Promise<void>) {
  const requested: string[] = [];
  await page.route("**/api/v1/sftp/transfers", (route) => route.fulfill({ json: transferList }));
  await page.route("**/api/v1/sftp/bastion/entries**", async (route) => {
    const path = new URL(route.request().url()).searchParams.get("path") || "/workspace";
    requested.push(path);
    if (path === "/workspace/projects") await waitForProjects;
    await route.fulfill({ json: {
      path,
      entries: path === "/workspace/projects" ? [entry("README.md", "file", path)] : rootEntries,
    } });
  });
  return requested;
}

async function openMobileFiles(page: Page) {
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "SFTP", exact: true }).tap();
  await page.getByRole("button", { name: "Host", exact: true }).tap();
  await page.getByRole("dialog").getByText("bastion", { exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
  await expect(page.getByRole("button", { name: "projects", exact: true })).toBeVisible();
}

async function expectInside(page: Page, selector: import("@playwright/test").Locator) {
  // Wait for entry animation and viewport resize layout to settle before
  // asserting the final bounds; visibility alone does not wait for transforms.
  await expect(async () => {
    const bounds = await selector.boundingBox();
    expect(bounds).not.toBeNull();
    if (bounds === null) return;
    const viewport = page.viewportSize();
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.y).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual((viewport?.width ?? 0) + 1);
    expect(bounds.y + bounds.height).toBeLessThanOrEqual((viewport?.height ?? 0) + 1);
  }).toPass({ timeout: 5000 });
}

test("mobile transfer details preserve the file list at 640px and 480px heights", async ({ page, installation }) => {
  await mockFiles(page);
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await expectInside(page, page.getByRole("navigation", { name: "Quick navigation" }));
  await capture(page, "mobile-home-390x640");
  await openMobileFiles(page);
  const list = page.getByTestId("sftp-file-list");
  const dock = page.getByRole("button", { name: "Expand Transfer Manager", exact: true });
  for (const height of [640, 480]) {
    await page.setViewportSize({ width: 390, height });
    // setViewportSize returns before the browser dispatches its resize event.
    // Measure only once the app has adopted the new visible viewport.
    await expect.poll(async () => (await page.locator(".sshc-app").boundingBox())?.height).toBe(height);
    await expect(dock).toBeVisible();
    const before = await list.boundingBox();
    expect(before?.height ?? 0).toBeGreaterThan(height === 640 ? 200 : 100);
    await capture(page, `mobile-files-390x${height}`);
    await dock.tap();
    const sheet = page.getByRole("dialog", { name: "Transfer Manager", exact: true });
    await expect(sheet).toBeVisible();
    await expectInside(page, sheet);
    const during = await list.boundingBox();
    expect(during?.height).toBeCloseTo(before?.height ?? 0, 0);
    expect(during?.y).toBeCloseTo(before?.y ?? 0, 0);
    await expect(sheet.getByText("reports.zip", { exact: true }).first()).toBeVisible();
    await capture(page, `mobile-transfers-390x${height}`);
    if (height === 640) {
      const handled = await page.evaluate(() => !window.dispatchEvent(new Event("sshc-android-back", { cancelable: true })));
      expect(handled).toBe(true);
    } else {
      await sheet.getByRole("button", { name: "Close Transfer Manager", exact: true }).tap();
    }
    await expect(sheet).toHaveCount(0);
    await expect(dock).toBeFocused();
    expect((await list.boundingBox())?.height).toBeCloseTo(before?.height ?? 0, 0);
  }
});

test("home cards connect with one tap and their menus stay inside the cards", async ({ page, installation }) => {
  const requested: unknown[] = [];
  await page.route("**/api/v1/terminal/sessions", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    requested.push(route.request().postDataJSON());
    await route.abort("failed");
  });
  await openApplication(page, installation);
  const list = page.getByRole("list", { name: "Available connections" });
  for (const height of [844, 640]) {
    await page.setViewportSize({ width: 390, height });
    for (const alias of ["bastion", "nas"]) {
      const card = list.getByRole("listitem").filter({ hasText: alias });
      await card.scrollIntoViewIfNeeded();
      const connect = card.getByRole("button", { name: new RegExp(`^Connect to ${alias}\\.`) });
      const menu = card.getByRole("button", { name: `Actions for ${alias}`, exact: true });
      await expect(card.getByRole("button")).toHaveCount(2);
      await expect(async () => {
        const outer = await card.boundingBox();
        const action = await menu.boundingBox();
        expect(outer).not.toBeNull();
        expect(action).not.toBeNull();
        if (outer === null || action === null) return;
        expect(action.height).toBeGreaterThanOrEqual(44);
        expect(action.width).toBeGreaterThanOrEqual(44);
        expect(action.x).toBeGreaterThanOrEqual(outer.x);
        expect(action.y).toBeGreaterThanOrEqual(outer.y);
        expect(action.x + action.width).toBeLessThanOrEqual(outer.x + outer.width);
        expect(action.y + action.height).toBeLessThanOrEqual(outer.y + outer.height);
      }).toPass();
      // Trial performs hit testing without starting a connection to the fixture host.
      await connect.tap({ trial: true });
      await menu.tap();
      await expect(page.getByRole("menuitem", { name: "Open connection settings" })).toBeVisible();
      expect(requested).toEqual([]);
      await page.keyboard.press("Escape");
    }
  }
  await list.getByRole("button", { name: /^Connect to nas\./ }).tap();
  await expect.poll(() => requested).toEqual([{ kind: "ssh", alias: "nas" }]);
});

// holdRow keeps a finger on a row, as a long press does: the browser turns it into the same pointer and context menu
// events a phone sends.
async function holdRow(page: Page, row: Locator) {
  const box = (await row.boundingBox())!;
  const touch = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const session = await page.context().newCDPSession(page);
  await session.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [touch] });
  await page.waitForTimeout(longPressHeldMs);
  await session.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

test("one tap opens folders with immediate loading feedback and a long press enters selection mode", async ({ page, installation }) => {
  let finishProjects: (() => void) | undefined;
  const projectsGate = new Promise<void>((resolve) => { finishProjects = resolve; });
  const requested = await mockFiles(page, projectsGate);
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openMobileFiles(page);
  try {
    await page.getByRole("button", { name: "projects", exact: true }).tap();
    await expect.poll(() => requested.filter((path) => path === "/workspace/projects").length).toBe(1);
    const progress = page.getByRole("status").filter({ hasText: "Reading the remote directory…" });
    await expect(progress).toBeVisible();
    await expect(progress).toContainText("/workspace/projects");
    await expect(page.getByTestId("sftp-file-list")).toHaveAttribute("inert", "");
  } finally {
    finishProjects?.();
  }
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace/projects");
  await expect(page.getByTestId("sftp-file-list")).not.toHaveAttribute("inert");
  await page.getByRole("button", { name: "Parent directory", exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
  const callsBeforeSelect = requested.length;
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await holdRow(page, page.getByRole("button", { name: "projects", exact: true }));
  await expect(page.getByRole("checkbox", { name: "Select projects", exact: true })).toBeChecked();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "archives", exact: true }).tap();
  await expect(page.getByRole("checkbox", { name: "Select archives", exact: true })).toBeChecked();
  expect(requested).toHaveLength(callsBeforeSelect);
  await expect(page.getByRole("button", { name: "Actions for 2 selected items", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Clear selection", exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
  await expect(page.getByRole("checkbox")).toHaveCount(0);
});

test("the folder menu holds what is done to the folder, and the order is chosen at the head of the list", async ({ page, installation }) => {
  await mockFiles(page);
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openMobileFiles(page);

  await page.getByRole("button", { name: "Folder actions", exact: true }).tap();
  const sheet = page.getByRole("dialog", { name: "Folder actions", exact: true });
  for (const name of ["New folder", "Upload", "Copy full path"]) await expect(sheet.getByRole("menuitem", { name, exact: true })).toBeVisible();
  for (const name of [/sort/u, "Home directory", "Root directory", "Back", "Forward", "Refresh directory", "Select all entries"]) {
    await expect(sheet.getByRole("menuitem", { name })).toHaveCount(0);
  }
  await sheet.getByRole("button", { name: "Close", exact: true }).tap();

  await page.getByRole("button", { name: "Sorted by Name", exact: true }).tap();
  const sortSheet = page.getByRole("dialog", { name: "Sort", exact: true });
  await sortSheet.getByRole("menuitemradio", { name: /^Size/u }).tap();
  await expect(sortSheet).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Sorted by Size", exact: true })).toBeVisible();

  await page.getByTestId("sftp-current-path").tap();
  await page.getByRole("button", { name: "Root", exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/");
});

// mockTerminalSession stands in for one shell whose output is the given text, and returns what the page sends it.
// redraw is sent again each time the page resizes the terminal, as a full-screen program redraws its screen then.
async function mockTerminalSession(page: Page, output: string, redraw = ""): Promise<string[]> {
  const messages: string[] = [];
  await page.route("**/api/v1/terminal/sessions**", (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      return route.fulfill({ status: 201, json: { streamTicket: "mobile-demo-ticket" } });
    }
    return route.fulfill({ json: { sessions: [{ id: "mobile-demo-shell", kind: "shell", workspaceId: "default", title: "Demo shell", startedAt: "2026-09-12T00:00:00Z", state: "connected", problem: "", forwards: [] }], maxSessions: 12 } });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from(output + redraw));
    socket.onMessage((data) => {
      const message = typeof data === "string" ? data : data.toString("utf8");
      messages.push(message);
      if (redraw !== "" && message.startsWith('{"resize"')) socket.send(Buffer.from(redraw));
    });
  });
  return messages;
}

test("quick navigation and terminal keys remain available in portrait and touch landscape", async ({ page, installation }) => {
  const messages = await mockTerminalSession(page, "Mobile terminal demo\r\nFiles: projects/  archives/  report.txt\r\n$ ");
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Quick navigation" });
  await navigation.getByRole("link", { name: "Connections", exact: true }).tap();
  await expect(page).toHaveURL(/\/connections\/servers$/);
  await expect(navigation.getByRole("link", { name: "Connections", exact: true })).toHaveAttribute("aria-current", "page");
  await navigation.getByRole("link", { name: "Terminal", exact: true }).tap();
  await expect(page).toHaveURL(/\/terminal$/);
  const keys = page.getByLabel("On-screen keys", { exact: true });
  await expect(keys).toBeVisible();
  await expect(page.getByRole("region", { name: "Terminal for Demo shell" })).toContainText("Mobile terminal demo");
  await capture(page, "mobile-terminal-390x640");
  // On the terminal the screen's own top bar replaces the bottom tabs, as in the design; its
  // ‹ opens the navigation, whose Home brings the tabs back.
  await expect(navigation).toHaveCount(0);
  const openNavigation = page.getByRole("button", { name: "Navigation", exact: true });
  for (const viewport of [{ width: 390, height: 640 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport);
    await expect(keys).toBeVisible();
    await expectInside(page, keys);
    await expectInside(page, openNavigation);
    await keys.getByRole("button", { name: "Up arrow", exact: true }).tap();
    await expect.poll(() => messages.some((message) => message.includes("\u001b[A"))).toBe(true);
    messages.length = 0;
  }
  await capture(page, "mobile-terminal-844x390");
  await openNavigation.tap();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Home", exact: true }).tap();
  await expect(page).toHaveURL(new URL("/", installation.url).toString());
  await expect(navigation.getByRole("link", { name: "Home", exact: true })).toHaveAttribute("aria-current", "page");
});

test("a phone on its side moves the bottom bar to a rail at the left and leaves out an idle transfer dock", async ({ page, installation }) => {
  await mockFiles(page);
  // Registered last, so it answers before mockFiles' list: nothing is being transferred.
  await page.route("**/api/v1/sftp/transfers", (route) => route.fulfill({ json: { ...transferList, jobs: [] } }));
  await page.setViewportSize({ width: 844, height: 390 });
  await openApplication(page, installation);
  const bar = page.getByRole("navigation", { name: "Quick navigation" }).locator(":scope > div");
  await expect(async () => {
    const bounds = await bar.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds?.x ?? 99).toBeLessThan(16);
    expect(bounds?.width ?? 99).toBeLessThan(60);
    expect(bounds?.height ?? 0).toBeGreaterThan(250);
  }).toPass({ timeout: 5000 });
  const railEnd = ((await bar.boundingBox())?.x ?? 0) + ((await bar.boundingBox())?.width ?? 0);
  expect((await page.locator("main").boundingBox())?.x ?? 0).toBeGreaterThanOrEqual(railEnd);
  await capture(page, "mobile-home-844x390");
  await openMobileFiles(page);
  await expect(page.getByRole("button", { name: "Expand Transfer Manager", exact: true })).toHaveCount(0);
  await capture(page, "mobile-files-844x390");
});

test("a tablet held upright keeps the navigation open at the left and centres the bottom bar under the page", async ({ page, installation }) => {
  await mockTerminalSession(page, "Tablet terminal demo\r\n$ ");
  await page.setViewportSize({ width: 820, height: 1180 });
  await openApplication(page, installation);
  const primary = page.getByRole("navigation", { name: "Primary" });
  await expect(primary).toBeVisible();
  await expect.poll(async () => (await primary.boundingBox())?.x).toBe(0);
  const navigationEnd = (await primary.boundingBox())?.width ?? 0;
  expect(navigationEnd).toBe(280);
  await expect(page.getByRole("button", { name: "Navigation", exact: true })).toHaveCount(0);
  expect((await page.locator("main").boundingBox())?.x ?? 0).toBeGreaterThanOrEqual(navigationEnd);
  const bar = page.getByRole("navigation", { name: "Quick navigation" }).locator(":scope > div");
  const bounds = await bar.boundingBox();
  expect(bounds?.width ?? 999).toBeLessThanOrEqual(420);
  // Centred in the room beside the navigation.
  expect((bounds?.x ?? 0) + (bounds?.width ?? 0) / 2).toBeCloseTo(navigationEnd + (820 - navigationEnd) / 2, 0);
  await capture(page, "tablet-home-820x1180");
  // A session opened from the bar shows beside the navigation, with no ‹ to open it again.
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Terminal", exact: true }).tap();
  await expect(page.getByRole("region", { name: "Terminal for Demo shell" })).toContainText("Tablet terminal demo");
  await expect(primary).toBeVisible();
  await expect(page.getByRole("button", { name: "Navigation", exact: true })).toHaveCount(0);
  await capture(page, "tablet-terminal-820x1180");
});

// The terminal's own text field: focusing it raises the phone's keyboard.
async function terminalInputFocused(page: Page): Promise<boolean> {
  return page.evaluate(() => document.activeElement?.classList.contains("xterm-helper-textarea") === true);
}

async function openShellFromNavigation(page: Page) {
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "New session", exact: true }).tap();
  const dialog = page.getByRole("dialog", { name: "New session" });
  await dialog.getByRole("button", { name: /^Local shell, localhost/ }).tap();
  await expect(dialog).toBeHidden();
}

test("opening and closing sessions leaves the keyboard down until a tap types into the text box", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  // The navigation is closed (and hidden from the accessibility tree) while a terminal is shown.
  const navigation = page.getByRole("navigation", { name: "Primary", includeHidden: true });
  const closeButtons = navigation.getByRole("button", { name: /^Close /, includeHidden: true });
  for (const count of [1, 2]) {
    await openShellFromNavigation(page);
    await expect(closeButtons).toHaveCount(count);
    // Attaching to the stream finishes a moment after the session is shown; neither may take the focus.
    await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
    expect(await terminalInputFocused(page)).toBe(false);
  }

  // Closing the session being shown shows the other one.
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await closeButtons.last().tap();
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).tap();
  await expect(closeButtons).toHaveCount(1);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/);
  expect(await terminalInputFocused(page)).toBe(false);

  // A tap on the terminal types into the text box, where Japanese is composed before it is sent.
  await page.locator("[data-navigation-backdrop]").tap({ position: { x: 370, y: 300 } });
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  await expect(page.getByLabel("Text to send to the terminal", { exact: true })).toBeFocused();
  // The key bar's Ctrl hands the next key to the terminal itself. An emulated phone raises no keyboard, so
  // the page is told the one a real phone raises for the text box is up (useAppViewport).
  await page.evaluate(() => { document.documentElement.dataset.keyboardOpen = "true"; });
  await page.getByLabel("On-screen keys", { exact: true }).getByRole("button", { name: "Ctrl", exact: true }).tap();
  await expect.poll(() => terminalInputFocused(page)).toBe(true);
});

test("the key bar's ^C stops a running program in one tap", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  const sendButton = page.getByRole("button", { name: "Send", exact: true });
  await textBox.fill("sleep 60");
  await sendButton.tap();
  await expect(drawnRows(page).filter({ visible: true })).toContainText("sleep 60");

  await page.getByLabel("On-screen keys", { exact: true }).getByRole("button", { name: "Ctrl+C", exact: true }).tap();
  // Left running, the sleep would hold this line back for a minute, well past the wait below.
  await textBox.fill("echo stopped-$((40+2))");
  await sendButton.tap();
  await expect(drawnRows(page).filter({ visible: true })).toContainText("stopped-42", { timeout: 15_000 });
});

// pressAndroidBack does what MainActivity.navigateBack does with Android's Back, past the open dialog check:
// the page closes what it can or moves itself, otherwise the app goes back in the history, and from Home with
// nothing to close the app leaves (reported here as "left").
async function pressAndroidBack(page: Page): Promise<"handled" | "history" | "left"> {
  return page.evaluate(() => {
    const event = new Event("sshc-android-back", { cancelable: true });
    window.dispatchEvent(event);
    if (event.defaultPrevented) return "handled";
    if (location.pathname === "/" && !location.search) return "left";
    history.back();
    return "history";
  });
}

test("Android back goes Home from any section of the bottom bar, and leaves the app from Home", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  const bar = page.getByRole("navigation", { name: "Quick navigation" });
  // Switching between the sections of the bar does not make Back walk them again.
  for (const name of ["Connections", "SFTP", "Menu", "Connections"]) {
    await bar.getByRole("link", { name, exact: true }).tap();
  }
  await expect(page).toHaveURL(/\/connections\/servers$/);
  await page.getByRole("button", { name: /^bastion\b/ }).first().tap();
  await expect(page).toHaveURL(/\/connections\/servers\?/);

  // Inside a section Back goes one step back: from the opened connection to the list.
  expect(await pressAndroidBack(page)).toBe("history");
  await expect(page).toHaveURL(/\/connections\/servers$/);
  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(page).toHaveURL(/\/$/);
  expect(await pressAndroidBack(page)).toBe("left");
});

test("a phone keeps the notices behind a button, shows them over the terminal on a tap, and hides them with Back", async ({ page, installation }) => {
  const lost = {
    id: "notices-fixture", kind: "ssh", workspaceId: "default", alias: "gateway", title: "gateway",
    startedAt: "2026-11-02T00:00:00Z", state: "exited", problem: "", forwards: [],
    exited: { code: -1, signal: "", at: "2026-11-02T00:30:00Z", connectionLost: true },
  };
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "notices" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [lost], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from("before the drop\r\n")); });
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("button", { name: /^gateway/ }).first().tap();

  const screen = page.getByRole("region", { name: /^Terminal for / });
  const notices = screen.getByRole("button", { name: "Notices", exact: true });
  await expect(notices).toBeVisible();
  await expect(screen.getByText("The SSH connection was lost.")).toHaveCount(0);
  const rows = await drawnRowCount(page);

  await notices.tap();
  const card = screen.getByRole("region", { name: "The SSH connection was lost." });
  await expect(card).toBeFocused();
  await expect(card.getByRole("button", { name: "Reconnect", exact: true })).toBeVisible();
  // Shown over the terminal, the notices leave its rows as they were.
  expect(await drawnRowCount(page)).toBe(rows);

  const handled = await page.evaluate(() => !window.dispatchEvent(new Event("sshc-android-back", { cancelable: true })));
  expect(handled).toBe(true);
  await expect(card).toHaveCount(0);
  await expect(notices).toHaveAttribute("aria-expanded", "false");
  await expect(page).toHaveURL(/\/terminal$/);
});

test("a phone says a notice once, in its card, and the strip keeps only what the card does not say", async ({ page, installation }) => {
  const reconnecting = {
    id: "once-fixture", kind: "ssh", workspaceId: "default", alias: "gateway", title: "gateway",
    startedAt: "2026-11-02T00:00:00Z", state: "reconnecting", problem: "reconnect_failed", forwards: [],
    reconnect: { attempt: 2, limit: 5, retryAt: "2099-01-01T00:00:00Z", problem: "reconnect_failed" },
  };
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "once" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [reconnecting], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from("before the drop\r\n")); });
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("button", { name: /^gateway/ }).first().tap();

  const screen = page.getByRole("region", { name: /^Terminal for / });
  await screen.getByRole("button", { name: "Notices", exact: true }).tap();
  await expect(screen.getByRole("button", { name: "Stop reconnecting", exact: true })).toHaveCount(1);
  await expect(screen.locator("[data-notice-card]").getByRole("button", { name: "Stop reconnecting", exact: true })).toBeVisible();
  // Why the connection is in trouble is not in the card, so the strip still says it.
  await expect(screen.getByRole("alert")).toBeVisible();
});

test("Android's Back closes the terminal's search bar first, and leaves the terminal on the next press", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await screen.getByRole("button", { name: "More terminal actions" }).tap();
  await page.getByRole("menuitem", { name: "Find", exact: true }).tap();
  const search = screen.getByRole("textbox", { name: "Search terminal output" });
  await expect(search).toBeVisible();

  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(search).toHaveCount(0);
  await expect(page).toHaveURL(/\/terminal$/);

  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(page).toHaveURL(/\/$/);
});

test("Android's Back ends the SFTP search, the choosing of rows and the editing of the path before it leaves the page", async ({ page, installation }) => {
  await mockFiles(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openMobileFiles(page);

  await page.getByRole("button", { name: "Search files", exact: true }).tap();
  const filter = page.getByRole("searchbox");
  await expect(filter).toBeVisible();
  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(filter).toHaveCount(0);

  await chooseEntryOnPhone(page.getByRole("tabpanel"), "projects");
  const clear = page.getByRole("button", { name: "Clear selection" }).first();
  await expect(clear).toBeVisible();
  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(clear).toHaveCount(0);

  await page.getByRole("button", { name: "Edit path", exact: true }).tap();
  const path = page.getByRole("textbox", { name: "Remote path", exact: true });
  await expect(path).toBeVisible();
  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(path).toHaveCount(0);
  await expect(page).toHaveURL(/\/files$/);

  expect(await pressAndroidBack(page)).toBe("handled");
  await expect(page).toHaveURL(/\/$/);
});

test("the terminal's search says a bad pattern on a row of its own, and leaves the buttons their width", async ({ page, installation }) => {
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await screen.getByRole("button", { name: "More terminal actions" }).tap();
  await page.getByRole("menuitem", { name: "Find", exact: true }).tap();
  await screen.getByRole("button", { name: "Use regular expression" }).tap();
  await screen.getByRole("textbox", { name: "Search terminal output" }).fill("([");

  const message = screen.getByRole("status").filter({ hasText: "Invalid" });
  await expect(message).toBeVisible();
  const close = await screen.getByRole("button", { name: "Close search" }).boundingBox();
  const said = await message.boundingBox();
  if (close === null || said === null) throw new Error("the search bar is not on the screen");
  expect(said.y).toBeGreaterThanOrEqual(close.y + close.height);
  expect(said.width).toBeGreaterThan(close.width * 4);
});

// A short screen is what a phone has left over a keyboard, or on its side.
for (const size of [{ width: 390, height: 420 }, { width: 842, height: 390 }]) {
  test(`the terminal's menu stays on a ${size.width}x${size.height} screen and scrolls to its last item`, async ({ page, installation }) => {
    await page.setViewportSize(size);
    await openApplication(page, installation);
    await openShellFromNavigation(page);
    await page.getByRole("button", { name: "More terminal actions" }).tap();
    const menu = page.getByRole("menu", { name: "More terminal actions" });
    await expect(menu).toBeInViewport({ ratio: 1 });
    const last = menu.getByRole("menuitemcheckbox").last();
    await last.scrollIntoViewIfNeeded();
    await expect(last).toBeInViewport({ ratio: 1 });
  });
}

test("the terminal's quick commands open under the phone's top bar, not over it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  const more = screen.getByRole("button", { name: "More terminal actions" });
  await more.tap();
  await page.getByRole("menuitem", { name: "Quick Commands", exact: true }).tap();
  const panel = page.getByRole("dialog", { name: "Quick Commands" });
  await expect(panel).toBeVisible();
  const bar = await more.boundingBox();
  const opened = await panel.boundingBox();
  if (bar === null || opened === null) throw new Error("the panel or the top bar is not on the screen");
  expect(opened.y).toBeGreaterThanOrEqual(bar.y + bar.height);
});

// openLocalFolderOnPhone opens a folder of the engine's disk in the SFTP tab on the screen.
async function openLocalFolderOnPhone(page: Page, directory: string) {
  await page.getByRole("button", { name: "Host", exact: true }).tap();
  await page.getByRole("dialog").getByText("Local", { exact: true }).first().tap();
  await page.getByRole("button", { name: "Edit local path", exact: true }).tap();
  const path = page.getByRole("textbox", { name: "Engine filesystem path", exact: true });
  await path.fill(directory);
  await path.press("Enter");
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", directory);
}

test("a folder comparison fits a phone, with what each side has under the name", async ({ page, installation }) => {
  const longName = "a-long-file-name-that-does-not-fit-the-width-of-a-phone-at-all-and-has-nowhere-to-break.txt";
  for (const [side, contents] of [["left", "short"], ["right", "a different length"]] as const) {
    await installation.write(`compare/${side}/${longName}`, contents);
  }
  const under = join(installation.home, ".ssh", "compare");
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "SFTP", exact: true }).tap();
  await openLocalFolderOnPhone(page, join(under, "left"));
  await page.getByRole("button", { name: "New tab", exact: true }).tap();
  await page.getByRole("dialog").getByText("Local", { exact: true }).first().tap();
  await page.getByRole("button", { name: "Edit local path", exact: true }).tap();
  const path = page.getByRole("textbox", { name: "Engine filesystem path", exact: true });
  await path.fill(join(under, "right"));
  await path.press("Enter");

  await page.getByRole("button", { name: "Compare directories", exact: true }).tap();
  const choosing = page.getByRole("dialog", { name: "Choose tabs to compare" });
  // The folder comes first, where the closed list shows it, so the two tabs can be told apart.
  await expect(choosing.getByRole("option", { name: /^Local:left / })).toHaveCount(2);
  await expect(choosing.getByRole("option", { name: /^Local:right / })).toHaveCount(2);
  await choosing.getByRole("button", { name: "Compare", exact: true }).tap();

  const comparison = page.getByRole("dialog", { name: "Compare directories" });
  const row = comparison.getByRole("row", { name: new RegExp(longName) });
  await expect(row).toContainText("Changed");
  await expect(row).toContainText("5 B");
  const scroller = comparison.locator("table").locator("xpath=..");
  const sideways = await scroller.evaluate((element) => element.scrollWidth - element.clientWidth);
  expect(sideways).toBeLessThanOrEqual(1);
});

test("the SFTP editor keeps its Japanese buttons on one line on a phone, beside a long path", async ({ page, installation }) => {
  const longName = "a-long-file-name-that-does-not-fit-the-width-of-a-phone-at-all-and-has-nowhere-to-break.txt";
  await installation.write(`editing/${longName}`, "text\n");
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  // English does not break a word; Japanese breaks anywhere, so a squeezed button shows it.
  await changeDisplayLanguage(page, "ja");
  await page.getByRole("navigation", { name: "クイックナビゲーション" }).getByRole("link", { name: "SFTP", exact: true }).tap();
  await page.getByRole("button", { name: "接続先", exact: true }).tap();
  await page.getByRole("dialog").getByText("ローカル", { exact: true }).first().tap();
  await page.getByRole("button", { name: "ローカルパスを編集", exact: true }).tap();
  const path = page.getByRole("textbox", { name: "sshcエンジン側のファイルパス", exact: true });
  await path.fill(join(installation.home, ".ssh", "editing"));
  await path.press("Enter");
  await chooseEntryOnPhone(page.getByRole("tabpanel"), longName);
  await page.getByRole("button", { name: `${longName}の操作` }).tap();
  await page.getByRole("menuitem", { name: "ファイルを編集", exact: true }).tap();

  const close = page.getByRole("dialog", { name: new RegExp(longName) }).getByRole("button", { name: "閉じる", exact: true });
  await expect(close).toBeVisible();
  const lines = await close.evaluate((button) => {
    const range = document.createRange();
    range.selectNodeContents(button);
    return new Set([...range.getClientRects()].map((rectangle) => Math.round(rectangle.top))).size;
  });
  expect(lines).toBe(1);
});

test("the transfer manager's sheet shows the whole summary on a phone", async ({ page, installation }) => {
  const job = (id: string, overrides: Record<string, unknown>) => ({
    id: `fixture_${id}`, batchId: `batch-${id}`, batchName: `${id}.txt`, batchKind: "file", alias: "bastion",
    sourceAlias: "nas", sourcePath: `/source/${id}.txt`, operation: "copy", direction: "remote", kind: "file",
    name: `${id}.txt`, remotePath: `/destination/${id}.txt`, totalBytes: 4096, transferredBytes: 1024,
    bytesPerSecond: 0, remainingSeconds: -1, status: "paused", allowedActions: ["resume", "cancel"],
    attempt: 1, reconnectAttempt: 0, reconnectAt: "", problem: "", lastModified: 0,
    expectedRevision: "", sourceFingerprint: "", overwrite: false, downloadRevision: "", downloadParts: [],
    createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", ...overrides,
  });
  await page.route("**/api/v1/sftp/transfers", (route) => route.fulfill({ json: {
    maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: true,
    largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20,
    speedLimitBytesPerSecond: 0, autoReconnect: true, maxReconnectAttempts: 3,
    jobs: [
      job("running", { status: "running", bytesPerSecond: 1048576, allowedActions: ["pause", "cancel"] }),
      job("paused", {}),
      job("overwrite", { status: "needs_overwrite" }),
      job("reconnect", { status: "reconnecting", reconnectAttempt: 2 }),
      job("failed", { status: "failed", problem: "sftp_connection_lost", allowedActions: ["retry", "remove"] }),
    ],
  } }));
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "SFTP", exact: true }).tap();
  await page.getByRole("button", { name: "Expand Transfer Manager", exact: true }).tap();
  const sheet = page.getByRole("dialog", { name: "Transfer Manager" });
  const summary = sheet.locator("[id$='-summary']").first();
  await expect(summary).toBeVisible();
  const hidden = await summary.evaluate((element) => element.scrollWidth - element.clientWidth);
  expect(hidden).toBeLessThanOrEqual(1);
  const heading = sheet.getByRole("heading", { name: "Transfer Manager" });
  expect(await heading.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
});

test("a name with nowhere to break folds inside a dialog on a phone", async ({ page, installation }) => {
  const title = "x".repeat(120);
  const session = {
    id: "long-name-fixture", kind: "shell", workspaceId: "default", title,
    startedAt: "2026-11-02T00:00:00Z", state: "connected", problem: "", forwards: [],
  };
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "long-name" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", () => undefined);
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("button", { name: `Close ${title}` }).tap();
  const dialog = page.getByRole("dialog");
  const heading = dialog.getByRole("heading");
  await expect(heading).toBeVisible();
  const widths = await heading.evaluate((element) => ({ heading: element.scrollWidth, dialog: element.closest("[role=dialog]")?.clientWidth ?? 0 }));
  expect(widths.heading).toBeLessThanOrEqual(widths.dialog);
});

test("the search on a phone names no keys, and has a button to close it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("button", { name: /^Search everything/ }).first().tap();
  const palette = page.getByRole("dialog");
  await expect(palette.getByText("Type user@host to connect")).toBeVisible();
  await expect(palette.locator("kbd:visible")).toHaveCount(0);
  await palette.getByRole("button", { name: "Close search" }).tap();
  await expect(palette).toHaveCount(0);
});

test("the Android app asks for the notification permission once, behind the notices button, when a program first reports", async ({ page, installation }) => {
  const claude = {
    id: "permission-fixture", kind: "ssh", workspaceId: "default", alias: "gateway", title: "Claude Code",
    startedAt: "2026-11-04T00:00:00Z", state: "connected", problem: "", forwards: [],
    programStatus: { state: "idle", app: "claude-code", version: 1 },
  };
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "permission" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [claude], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from("$ claude\r\n")); });
  await page.addInitScript(() => {
    const android = window as unknown as { askedForPermission: number };
    android.askedForPermission = 0;
    const answers: Partial<NonNullable<Window["sshcAndroid"]>> = {
      notificationPermission: () => "default",
      requestNotificationPermission: () => { android.askedForPermission += 1; },
    };
    // Every other bridge call answers as an idle app would, as in support/androidApp.ts: the page calls more of
    // them than this test is about.
    Object.defineProperty(window, "sshcAndroid", {
      value: new Proxy(answers, { get: (target, name) => Reflect.get(target, name) ?? (() => "") }),
    });
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("button", { name: /^(gateway|Claude Code)/ }).first().tap();

  const screen = page.getByRole("region", { name: /^Terminal for / });
  const notices = screen.getByRole("button", { name: "Notices", exact: true });
  await expect(notices).toBeVisible();
  const card = screen.getByRole("region", { name: "Get notified when a program is waiting?" });
  await expect(card).toHaveCount(0);

  await notices.tap();
  await card.getByRole("button", { name: "Enable notifications", exact: true }).tap();
  expect(await page.evaluate(() => (window as unknown as { askedForPermission: number }).askedForPermission)).toBe(1);
  await expect(card).toHaveCount(0);
  await expect(notices).toHaveCount(0);

  // Answered once: Android still reports "default" after one refusal, and the app does not ask again.
  await page.reload();
  await expect(screen).toBeVisible();
  await expect(notices).toHaveCount(0);
});

test("Android back lets go of text selected in the terminal before it leaves the terminal", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
  await expect(page).toHaveURL(/\/terminal/);
  const terminal = page.url();

  // A long press selects through Android's own handles, which a test cannot raise; the selection they make is this one.
  await page.evaluate(() => { document.getSelection()!.selectAllChildren(document.querySelector(".sshc-select-overlay")!); });
  const selected = () => page.evaluate(() => document.getSelection()?.isCollapsed === false);
  expect(await selected()).toBe(true);

  expect(await pressAndroidBack(page)).toBe("handled");
  expect(await selected()).toBe(false);
  expect(page.url()).toBe(terminal);

  // Nothing is left to let go of: this one leaves.
  await pressAndroidBack(page);
  await expect(page).not.toHaveURL(terminal);
});

test("keeps what was written in the phone's text box once the session has ended, and says what to do", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });

  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  await textBox.fill("exit");
  await page.getByRole("button", { name: "Send", exact: true }).tap();
  await expect(page.getByRole("button", { name: "Press Enter", exact: true })).toHaveAttribute("aria-disabled", "true");

  await textBox.fill("a long request to Claude Code");
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toHaveAttribute("aria-disabled", "true");
  // A person can still press it, to learn why it does not send; Playwright would wait for it to be enabled.
  await send.tap({ force: true });

  await expect(textBox).toHaveValue("a long request to Claude Code");
  await expect(textBox).toBeEditable();
  await expect(page.getByText("This session has ended.")).toBeVisible();
});

// A made-up answer: no password of anyone's. It is also what must not be found anywhere after it is typed.
const typedSecret = "not-a-real-password";

// askForSecret is a command that asks without echoing, as sudo does, and then says how long the answer it read was.
const askForSecret = (question: string) => `printf '${question}'; read -s answer; echo; echo "read-\${#answer}"`;

const phoneViewport = { width: 390, height: 844 };
// An emulated phone raises no keyboard, so its screen is made as short as one would leave it (useAppViewport).
const screenAboveKeyboard = { width: 390, height: 500 };

async function openShellOnPhone(page: Page, installation: Installation) {
  await page.setViewportSize(phoneViewport);
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  const rows = drawnRows(page).filter({ visible: true });
  await expect(rows).toContainText(/[$#%>]/, { timeout: 20_000 });
  return {
    rows,
    textBox: page.getByLabel("Text to send to the terminal", { exact: true }),
    secretField: page.getByLabel("Password to send to the terminal", { exact: true }),
    send: page.getByRole("button", { name: "Send", exact: true }),
    pressEnter: page.getByRole("button", { name: "Press Enter", exact: true }),
    keyBar: page.getByLabel("On-screen keys", { exact: true }),
  };
}

// raiseKeyboardForTextBox taps the terminal as a person does, which types into the text box and raises the keyboard.
async function raiseKeyboardForTextBox(page: Page) {
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  await expect(page.getByLabel("Text to send to the terminal", { exact: true })).toBeFocused();
  await page.setViewportSize(screenAboveKeyboard);
  await expect(page.locator("html")).toHaveAttribute("data-keyboard-open", "true");
}

test("hides what is typed while a program asks for a password, and brings the text box back once it is answered", async ({ page, installation }) => {
  const requestBodies: string[] = [];
  page.on("request", (request) => { requestBodies.push(request.postData() ?? ""); });
  const { rows, textBox, secretField, send } = await openShellOnPhone(page, installation);
  await expect(textBox).toBeVisible();
  await expect(secretField).toBeHidden();
  await raiseKeyboardForTextBox(page);

  await textBox.fill(askForSecret("Password: "));
  await send.tap();

  await expect(secretField).toBeVisible();
  await expect(secretField).toHaveAttribute("type", "password");
  await expect(secretField).toBeFocused();
  await expect(textBox).toBeHidden();

  await page.keyboard.type(typedSecret);
  await expect(secretField).toHaveValue(typedSecret);
  const stored = await page.evaluate(() => JSON.stringify([{ ...localStorage }, { ...sessionStorage }]));
  expect(stored).not.toContain(typedSecret);

  // Enter sends even though it starts a new line in the text box (the default).
  await page.keyboard.press("Enter");
  await expect(rows).toContainText(`read-${typedSecret.length}`);
  await expect(textBox).toBeVisible();
  await expect(textBox).toBeFocused();
  await expect(textBox).toHaveValue("");
  await expect(secretField).toBeHidden();
  await expect(secretField).toHaveValue("");
  await expect(rows).not.toContainText(typedSecret);
  expect(await page.evaluate(() => JSON.stringify([{ ...localStorage }, { ...sessionStorage }]))).not.toContain(typedSecret);
  expect(requestBodies.join("\n")).not.toContain(typedSecret);

  // Japanese, and the sentence a shell prints: the box goes back and forth with what the cursor is after.
  await textBox.fill(askForSecret("[sudo] deploy のパスワード: "));
  await send.tap();
  await expect(secretField).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(textBox).toBeVisible();
});

test("leaves the text box as it is at a shell's prompt and when the word is only in a sentence", async ({ page, installation }) => {
  const { rows, textBox, secretField, send, pressEnter } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);

  await textBox.fill("echo your password: is safe; printf 'password: done'; read -s answer");
  await send.tap();
  await expect(rows).toContainText("password: done");
  await expect(textBox).toBeVisible();
  await expect(secretField).toBeHidden();
  await pressEnter.tap();

  await textBox.fill("echo password: && echo finished");
  await send.tap();
  await expect(rows).toContainText("finished");
  await expect(textBox).toBeVisible();
  await expect(secretField).toBeHidden();
  await expect(textBox).toBeFocused();
});

test("keeps what was being written in the text box while a password is asked for, and neither sends nor hides it", async ({ page, installation }) => {
  const { rows, textBox, secretField, send } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);
  // The prompt of the isolated shell shows no real names in the screenshots.
  await textBox.fill("PS1='demo$ '; clear");
  await send.tap();
  await expect(rows).toContainText("demo$");
  await textBox.fill(`sleep 2; ${askForSecret("Password: ")}`);
  await send.tap();
  const request = "a long request\nto Claude Code";
  await textBox.fill(request);
  await expect(textBox).toBeFocused();
  const heightWithTwoLines = (await textBox.boundingBox())!.height;
  await capture(page, "composer-draft-before-question");

  await expect(secretField).toBeVisible();
  await expect(secretField).toBeFocused();
  await expect(secretField).toHaveValue("");
  await page.keyboard.type("short");
  await capture(page, "composer-draft-during-question");
  await send.tap();

  await expect(rows).toContainText("read-5");
  await expect(textBox).toBeVisible();
  await expect(textBox).toBeFocused();
  await expect(textBox).toHaveValue(request);
  expect(await textBox.evaluate((field: HTMLTextAreaElement) => field.selectionStart)).toBe(request.length);
  expect((await textBox.boundingBox())!.height).toBe(heightWithTwoLines);
  await expect(rows).not.toContainText("long request");
  await capture(page, "composer-draft-after-question");
});

test("drops what was typed for a password when the question goes away unanswered", async ({ page, installation }) => {
  const { rows, textBox, secretField, send, keyBar } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);
  await textBox.fill(askForSecret("Password: "));
  await send.tap();
  await expect(secretField).toBeVisible();
  await page.keyboard.type(typedSecret);

  await keyBar.getByRole("button", { name: "Ctrl+C", exact: true }).tap();

  await expect(textBox).toBeVisible();
  await expect(textBox).toHaveValue("");
  await expect(secretField).toBeHidden();
  await expect(secretField).toHaveValue("");
  await expect(rows).not.toContainText(typedSecret);
});

test("drops what was typed for a password when the session ends at the question", async ({ page, installation }) => {
  const { textBox, secretField, send, pressEnter } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);
  await textBox.fill("printf 'Password: '; sleep 2; exit");
  await send.tap();
  await expect(secretField).toBeVisible();
  await page.keyboard.type(typedSecret);

  await expect(pressEnter).toHaveAttribute("aria-disabled", "true", { timeout: 20_000 });
  await expect(textBox).toBeVisible();
  await expect(textBox).toHaveValue("");
  await expect(secretField).toHaveValue("");
});

test("switching to typing into the terminal drops what was typed for a password, and the password field comes back empty", async ({ page, installation }) => {
  const { rows, textBox, secretField, send, keyBar } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);
  await textBox.fill(askForSecret("Password: "));
  await send.tap();
  await expect(secretField).toBeVisible();
  await page.keyboard.type("half");

  const toggle = keyBar.getByRole("button", { name: "Type into the terminal", exact: true });
  await toggle.tap();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await expect(secretField).toHaveCount(0);
  await expect.poll(() => terminalInputFocused(page)).toBe(true);
  await page.keyboard.type("abc");

  await toggle.tap();
  await expect(secretField).toBeVisible();
  await expect(secretField).toHaveValue("");
  await expect(secretField).toBeFocused();
  await page.keyboard.type("de");
  await page.keyboard.press("Enter");

  await expect(rows).toContainText("read-5");
  await expect(textBox).toBeVisible();
});

test("a question that appears with the keyboard down leaves it down, and a tap on the terminal types the answer", async ({ page, installation }) => {
  const { rows, textBox, secretField, send } = await openShellOnPhone(page, installation);
  await raiseKeyboardForTextBox(page);
  // Back closes the keyboard and leaves the focus in the field; going by the field's focus alone would raise it again.
  await textBox.fill(`sleep 1; ${askForSecret("Password: ")}`);
  await send.tap();
  await page.setViewportSize(phoneViewport);
  await expect(page.locator("html")).toHaveAttribute("data-keyboard-open", "false");
  await expect(textBox).toBeFocused();

  await expect(secretField).toBeVisible();
  await expect(secretField).not.toBeFocused();
  await expect(textBox).not.toBeFocused();
  expect(await terminalInputFocused(page)).toBe(false);

  const screenBox = (await rows.boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  await expect(secretField).toBeFocused();
  await page.keyboard.type("short");
  await page.keyboard.press("Enter");
  await expect(rows).toContainText("read-5");
});

// The suite has no SSH server to log in to, so the engine's side is played: the session that waits for an answer, and
// the question as internal/sshclient/auth.go writes it.
test("answers sshc's own question for a password with what was typed, as it was typed", async ({ page, installation }) => {
  const asking = {
    id: "password-question-fixture",
    kind: "ssh", workspaceId: "default",
    alias: "web-01",
    title: "web-01",
    startedAt: "2026-11-04T00:00:00Z",
    state: "connecting",
    awaitingPrompt: true,
    problem: "",
    progress: { phase: "authenticating", alias: "web-01", hostName: "10.0.2.15", user: "deploy", hop: 1, hops: 1 },
    forwards: [],
  };
  let session: Record<string, unknown> = asking;
  const sent: string[] = [];
  let stream: WebSocketRoute | undefined;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "password-question" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    stream = socket;
    socket.onMessage((message) => { if (typeof message !== "string") sent.push(message.toString("utf8")); });
    socket.send(Buffer.from("Password for deploy@10.0.2.15:22 (web-01): "));
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const secretField = page.getByLabel("Password to send to the terminal", { exact: true });
  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  await expect(drawnRows(page).filter({ visible: true })).toContainText("Password for");
  await expect(secretField).toBeVisible();
  await capture(page, "composer-sshc-password-question-empty");
  await secretField.fill(typedSecret);
  await capture(page, "composer-sshc-password-question-typed");
  const send = page.getByRole("button", { name: "Send", exact: true });
  await expect(send).toHaveAttribute("aria-disabled", "false");
  await send.tap();

  await expect.poll(() => sent.join("")).toContain(`${typedSecret}\r`);
  expect(sent.join("")).not.toContain("\x1b[200~");
  await expect(secretField).toHaveValue("");
  session = { ...asking, state: "connected", awaitingPrompt: false };
  stream?.send(Buffer.from("\r\ndeploy@web-01:~$ "));
  await expect(textBox).toBeVisible();
  await expect(secretField).toBeHidden();
  await capture(page, "composer-after-the-answer");
});

test("a finger moves the shell's scrollback by the pixel and settles it on a row", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  await textBox.fill("seq 1 200");
  await page.getByRole("button", { name: "Send", exact: true }).tap();
  await expect(drawnRows(page).filter({ visible: true })).toContainText("200");

  // A finger pulls the text down by a third of a row: older lines, the rows drawn that far down.
  const host = page.locator("[data-terminal-host]").first();
  const moved = await page.evaluate(() => {
    const surface = document.querySelector<HTMLElement>(".sshc-select-overlay")!;
    const rows = Number.parseFloat(getComputedStyle(surface).lineHeight);
    const touch = (type: string, y: number) => {
      const finger = new Touch({ identifier: 1, target: surface, clientX: 200, clientY: y });
      surface.dispatchEvent(new TouchEvent(type, { touches: type === "touchend" ? [] : [finger], changedTouches: [finger], bubbles: true, cancelable: true }));
    };
    const bottomRow = (surface.textContent ?? "").split("\n").filter((line) => line.trim() !== "").at(-1)?.trimEnd() ?? "";
    touch("touchstart", 400);
    touch("touchmove", 400 + rows / 3);
    const host = document.querySelector<HTMLElement>("[data-terminal-host]")!;
    const edge = host.querySelector<HTMLElement>("[data-scrollback-edge-row]")!;
    return { rows, bottomRow, transform: host.style.transform, edge: edge.style.display === "block" ? edge.textContent : null };
  });
  // The rows from the one before, drawn two thirds of a row up: the text sits a third of a row lower, as the finger
  // moved it. The row that was at the bottom is now the one under the rows, drawn there.
  expect(moved.transform).toBe(`translateY(${-(moved.rows * 2) / 3}px)`);
  expect(moved.edge).toBe(moved.bottomRow);

  // Let go slowly: the rows settle on the nearer one.
  await page.waitForTimeout(150);
  await page.evaluate(() => {
    const surface = document.querySelector<HTMLElement>(".sshc-select-overlay")!;
    const finger = new Touch({ identifier: 1, target: surface, clientX: 200, clientY: 410 });
    surface.dispatchEvent(new TouchEvent("touchend", { touches: [], changedTouches: [finger], bubbles: true, cancelable: true }));
  });
  await expect(host).toHaveAttribute("style", /^(?!.*translateY)/);
});

test("coming back to the app after typing leaves the keyboard down", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  await expect(textBox).toBeFocused();

  // Coming back to the page holds the app while the vault is rechecked; the field that had the focus is not given
  // it back, which on a phone would raise the keyboard (Android keeps the focus in the field when its keyboard is
  // closed with Back).
  const app = page.locator(".sshc-app > .contents");
  // On a phone the recheck takes long enough for the browser to move the focus off the inert field. Answered at
  // once, the app could be shown again before the browser does, and the focus would never leave.
  await page.route("**/api/v1/passwords", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 300));
    await route.fallback();
  });
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
  await expect(app).toHaveAttribute("inert");
  await expect(app).not.toHaveAttribute("inert");
  await expect(textBox).not.toBeFocused();
  expect(await terminalInputFocused(page)).toBe(false);
});

// coveredAtScreenCentre says whether something is laid over the page in the middle of the screen. An inert page takes
// no hits, so without a cover the point lands on the app's own frame.
function coveredAtScreenCentre(page: Page): Promise<boolean> {
  return page.evaluate(() => (document.elementFromPoint(window.innerWidth / 2, window.innerHeight / 2)?.closest(".fixed") ?? null) !== null);
}

test("coming back to the app keeps the screen readable while the vault is checked, with a small mark", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  const homeHeading = page.getByRole("heading", { name: "Home", includeHidden: true }).first();
  await expect(homeHeading).toBeVisible();

  let answer = () => {};
  const answered = new Promise<void>((resolve) => { answer = resolve; });
  await page.route("**/api/v1/passwords", async (route) => {
    await answered;
    await route.fallback();
  });
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));

  const app = page.locator(".sshc-app > .contents");
  await expect(app).toHaveAttribute("inert");
  const mark = page.getByRole("status").filter({ hasText: "Checking that the vault is still unlocked" });
  await expect(mark).toBeVisible();
  // The page is still what is on screen: nothing is laid over it but the mark at the top.
  expect((await mark.boundingBox())!.height).toBeLessThan(48);
  expect(await coveredAtScreenCentre(page)).toBe(false);
  await capture(page, "vault-recheck");

  answer();
  await expect(app).not.toHaveAttribute("inert");
  await expect(mark).toHaveCount(0);
});

test("a vault check that fails on coming back hides the app until a retry succeeds", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await page.route("**/api/v1/passwords", (route) => route.abort());
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));

  const notice = page.getByText("Protected content remains hidden while sshc retries.");
  await expect(notice).toBeVisible();
  expect(await coveredAtScreenCentre(page)).toBe(true);
  await capture(page, "vault-recheck-failed");

  await page.unroute("**/api/v1/passwords");
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(notice).toHaveCount(0);
  await expect(page.locator(".sshc-app > .contents")).not.toHaveAttribute("inert");
  expect(await coveredAtScreenCentre(page)).toBe(false);
});

test("a five-digit port on a phone is not hidden under the button that resets it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Connections", exact: true }).tap();
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).tap();
  const port = page.getByLabel("Port", { exact: true });
  await port.fill("22298");
  const reset = page.getByRole("button", { name: "Use inherited/default port", exact: true });
  await expect(reset).toBeVisible();
  // Where the value's text ends must be left of where the button starts.
  const { textEnd, buttonStart } = await port.evaluate((input: HTMLInputElement) => {
    const style = getComputedStyle(input);
    const context = document.createElement("canvas").getContext("2d")!;
    context.font = `${style.fontSize} ${style.fontFamily}`;
    const box = input.getBoundingClientRect();
    const button = input.parentElement!.querySelector("button")!.getBoundingClientRect();
    return { textEnd: box.left + Number.parseFloat(style.paddingLeft) + context.measureText(input.value).width, buttonStart: button.left };
  });
  expect(textEnd).toBeLessThanOrEqual(buttonStart);
});

test("the navigation on a phone links only Home and gives the sessions the height", async ({ page, installation }) => {
  const sessions = Array.from({ length: 12 }, (_, index) => ({
    id: `phone-${index + 1}`, kind: "shell", workspaceId: "default", title: `Session ${index + 1}`,
    startedAt: "2026-10-29T00:00:00Z", state: "connected", problem: "", forwards: [],
  }));
  await page.route("**/api/v1/terminal/sessions", (route) => route.request().method() === "GET"
    ? route.fulfill({ json: { sessions, maxSessions: 50 } })
    : route.fallback());
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Navigation", exact: true }).tap();
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByText("Session 12", { exact: true })).toBeAttached();
  // The bar at the bottom holds the other sections and Menu.
  await expect(navigation.getByRole("list", { name: "Start" }).getByRole("link")).toHaveText(["Home"]);
  await expect(navigation.getByRole("link", { name: "Menu", exact: true })).toHaveCount(0);
  // The sessions' list starts right under Home and runs down to the run target.
  const list = navigation.locator("[data-navigation-scroll]");
  const [homeBox, listBox] = await Promise.all([navigation.getByRole("link", { name: "Home", exact: true }).boundingBox(), list.boundingBox()]);
  expect(listBox!.y - (homeBox!.y + homeBox!.height)).toBeLessThanOrEqual(4);
  expect(listBox!.height).toBeGreaterThanOrEqual(320);
  await capture(page, "mobile-navigation-390x640");
});

test("a page's button in the top bar is drawn small and still answers a tap just below it", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await page.goto(new URL("/connections", page.url()).href);
  const topBar = page.locator("[data-app-header]");
  const add = topBar.getByRole("button", { name: "Add connection", exact: true });
  await expect(add).toBeVisible();
  const bar = (await topBar.boundingBox())!;
  const button = (await add.boundingBox())!;
  // The bar is 48px tall; the button leaves room around it instead of filling it.
  expect(button.height).toBeLessThanOrEqual(32);
  expect(button.height).toBeLessThan(bar.height - 12);
  // The place to press still reaches past the drawn button, as tall as a touch target should be.
  await page.touchscreen.tap(button.x + button.width / 2, button.y + button.height + 6);
  await expect(page.getByRole("form", { name: "New connection" })).toBeVisible();
});

test("a new connection takes the phone's screen in place of the list and opens its settings once created", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await page.goto(new URL("/connections", page.url()).href);
  await page.locator("[data-app-header]").getByRole("button", { name: "Add connection", exact: true }).tap();

  const form = page.getByRole("form", { name: "New connection" });
  await expect(form).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeHidden();
  await form.getByLabel("Connection name").fill("phone-node");
  await form.getByLabel("Host name or IP address").fill("phone.example");
  await form.getByRole("textbox", { name: "Connection password", exact: true }).fill("phone node password");
  await form.getByRole("button", { name: "Create connection" }).tap();

  await expect(page.getByRole("heading", { name: "phone-node", exact: true })).toBeVisible();
  await expect(form).toHaveCount(0);
});

// textFieldFocused says a phone's keyboard would be up, or come up on the next tap anywhere.
async function textFieldFocused(page: Page): Promise<boolean> {
  return page.evaluate(() => document.activeElement?.matches("input:not([type=checkbox]):not([type=radio]), textarea") === true);
}

test("a new connection opens with the keyboard down and its fields ask for Latin letters", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.goto(new URL("/connections", page.url()).href);
  await page.locator("[data-app-header]").getByRole("button", { name: "Add connection", exact: true }).tap();
  const form = page.getByRole("form", { name: "New connection" });
  await expect(form).toBeVisible();
  expect(await textFieldFocused(page)).toBe(false);

  const name = form.getByLabel("Connection name");
  for (const field of [name, form.getByLabel("Host name or IP address"), form.getByLabel("User", { exact: true })]) {
    await expect(field).toHaveAttribute("inputmode", "email");
    await expect(field).toHaveAttribute("autocapitalize", "off");
  }
  await name.tap();
  await expect(name).toBeFocused();
});

test("a form or a dialog opens with the keyboard down and closes without bringing it up", async ({ page, installation }) => {
  await openApplication(page, installation);

  // A form in the page.
  await page.goto(new URL("/vault/passwords", page.url()).href);
  await page.locator("[data-app-header]").getByRole("button", { name: "Add", exact: true }).tap();
  const credential = page.getByRole("region", { name: "Add an account password" });
  await expect(credential).toBeVisible();
  expect(await textFieldFocused(page)).toBe(false);

  // A dialog whose first control is a search field.
  await page.getByRole("button", { name: "Navigation", exact: true }).first().tap();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "Search everything" }).tap();
  const palette = page.getByRole("dialog", { name: /^Search sessions/ });
  await expect(palette).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(palette).toBeHidden();
  expect(await textFieldFocused(page)).toBe(false);

  // A dialog that asks for a value on its own: the field takes a tap.
  await page.goto(new URL("/settings/keybar", page.url()).href);
  await page.getByRole("button", { name: "Make a key", exact: true }).tap();
  const making = page.getByRole("dialog", { name: "Your own key" });
  await expect(making).toBeFocused();
  await making.getByLabel("Text on the key").tap();
  await expect(making.getByLabel("Text on the key")).toBeFocused();
});

test("a control that turns into its field on a tap still takes the focus", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.goto(new URL("/connections", page.url()).href);
  await page.locator("[data-app-header]").getByRole("button", { name: "Add connection", exact: true }).tap();
  await page.getByRole("button", { name: "New group", exact: true }).tap();
  await expect(page.getByLabel("Name of the new group")).toBeFocused();
});

// The widest a key can be with all ten keys of the default row on a phone 412px wide (KeyBar.tsx).
const keyBarKeyMinimumWidth = 39;

test("all the keys of the key bar show on a phone 412px wide, each as tall as a finger", async ({ page, installation }) => {
  await mockTerminalSession(page, "$ ");
  await page.setViewportSize({ width: 412, height: 800 });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Terminal", exact: true }).tap();
  const keys = page.getByLabel("On-screen keys", { exact: true });
  await expect(keys).toBeVisible();
  const buttons = await keys.getByRole("button").all();
  expect(buttons).toHaveLength(10);
  for (const button of buttons) {
    await expectInside(page, button);
    const box = (await button.boundingBox())!;
    expect(box.width).toBeGreaterThanOrEqual(keyBarKeyMinimumWidth);
    expect(box.height).toBeGreaterThanOrEqual(44);
  }
  // Nothing is hidden behind 「…」, which the row would run under if it were too wide for the screen.
  const more = (await keys.getByRole("button", { name: "Extra keys", exact: true }).boundingBox())!;
  const right = (await keys.getByRole("button", { name: "Right arrow", exact: true }).boundingBox())!;
  expect(right.x + right.width).toBeLessThanOrEqual(more.x + 1);
});

test("on a narrower phone the row of keys scrolls sideways and 「…」 stays in sight", async ({ page, installation }) => {
  const messages = await mockTerminalSession(page, "Mobile terminal demo\r\n$ ");
  await page.setViewportSize({ width: 360, height: 640 });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Terminal", exact: true }).tap();
  const keys = page.getByLabel("On-screen keys", { exact: true });
  await expect(keys).toBeVisible();
  for (const button of await keys.getByRole("button").all()) {
    expect((await button.boundingBox())!.width).toBeGreaterThanOrEqual(keyBarKeyMinimumWidth);
  }
  const more = keys.getByRole("button", { name: "Extra keys", exact: true });
  await expectInside(page, more);

  // The row keeps the swipe for itself: the navigation stays closed, 「…」 stays put, and the hidden key comes into sight.
  const right = keys.getByRole("button", { name: "Right arrow", exact: true });
  const moreBefore = (await more.boundingBox())!;
  const rightBefore = (await right.boundingBox())!;
  expect(rightBefore.x + rightBefore.width).toBeGreaterThan(moreBefore.x);
  const y = (await keys.boundingBox())!.y + 30;
  await waitForPageFadeToEnd(page);
  await swipe(page, { x: 250, y }, { x: 40, y });
  await expect(page.locator("[data-app-navigation]")).toHaveAttribute("data-navigation-open", "false");
  await expect.poll(async () => { const box = (await right.boundingBox())!; return box.x + box.width; }).toBeLessThanOrEqual(moreBefore.x + 1);
  expect((await more.boundingBox())!.x).toBe(moreBefore.x);

  await right.tap();
  await expect.poll(() => messages.some((message) => message.includes("\u001b[C"))).toBe(true);
});

test("the Connection layout switch answers a tap just outside what is drawn", async ({ page, installation }) => {
  await openApplication(page, installation);
  const list = page.getByRole("button", { name: "List", exact: true });
  const box = (await list.boundingBox())!;
  expect(box.height).toBeLessThan(30);
  await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height + 8);
  await expect(list).toHaveAttribute("aria-pressed", "true");
  const panels = page.getByRole("button", { name: "Panels", exact: true });
  const panelsBox = (await panels.boundingBox())!;
  await page.touchscreen.tap(panelsBox.x + panelsBox.width / 2, panelsBox.y - 8);
  await expect(panels).toHaveAttribute("aria-pressed", "true");
});

test("More details and a slider are as tall as a finger", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.goto(new URL("/settings/vault", page.url()).href);
  const summary = page.getByText("More details", { exact: true }).first();
  await expect(summary).toBeVisible();
  const box = (await summary.boundingBox())!;
  expect(box.height).toBeGreaterThanOrEqual(44);
  await summary.tap();
  await expect(summary.locator("xpath=ancestor::details")).toHaveAttribute("open", "");

  await page.goto(new URL("/settings/notifications", page.url()).href);
  const slider = page.getByRole("slider").first();
  await expect(slider).toBeVisible();
  expect((await slider.boundingBox())!.height).toBeGreaterThanOrEqual(44);
});

test("a tap on the line being typed at a prompt moves the shell's cursor there", async ({ page, installation }) => {
  // Shell integration marks where the prompt and the typed text begin (OSC 133;A and B).
  const messages = await mockTerminalSession(page, "\x1b]133;A\x07$ \x1b]133;B\x07echo 0123456789");
  await openApplication(page, installation);
  await page.goto(new URL("/terminal", page.url()).href);
  const typed = drawnSpan(page, "echo 0123456789").last();
  await expect(typed).toBeVisible();
  const box = await typed.boundingBox();
  const text = (await typed.textContent()) ?? "";
  if (box === null) throw new Error("the typed line is not drawn");
  const cellWidth = box.width / text.length;

  await page.touchscreen.tap(box.x + (text.indexOf("5") + 0.5) * cellWidth, box.y + box.height / 2);

  // From after the 9 back to the 5: one arrow key for each of 9, 8, 7, 6 and 5.
  await expect.poll(() => messages.join("")).toContain("\x1b[D".repeat(5));
  expect(messages.join("")).not.toContain("\x1b[D".repeat(6));
});

// openShellPrinting opens a shell that printed output, which names files on the engine's own disk: the
// preview of a printed path reads the real file.
async function openShellPrinting(page: Page, installation: Installation, output: string) {
  await page.route("**/api/v1/terminal/sessions**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/stream")) return route.fulfill({ status: 201, json: { streamTicket: "mobile-path-ticket" } });
    if (path.endsWith("/directory")) return route.fulfill({ json: { path: installation.home } });
    return route.fulfill({ json: { sessions: [{ id: "mobile-path-shell", kind: "shell", workspaceId: "default", title: "claude", startedAt: "2026-09-12T00:00:00Z", state: "connected", problem: "", forwards: [] }], maxSessions: 12 } });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from(output));
  });
  await openApplication(page, installation);
  await page.goto(new URL("/terminal", page.url()).href);
}

async function printedBounds(page: Page, text: string) {
  const printed = drawnSpan(page, text).last();
  await expect(printed).toBeVisible();
  const bounds = await printed.boundingBox();
  if (bounds === null) throw new Error("the printed path is not on screen");
  return bounds;
}

test("a tap on a file path the terminal printed opens its preview at once", async ({ page, installation }) => {
  const imagePath = join(installation.home, "shot.png");
  test.skip(!imagePath.startsWith("/"), "terminal paths are detected only when they start with /");
  await writeFile(imagePath, samplePNG());
  await openShellPrinting(page, installation, `Saved \x1b[36m${imagePath}\x1b[0m\r\n$ `);
  const bounds = await printedBounds(page, "shot.png");

  await page.touchscreen.tap(bounds.x + bounds.width - 4, bounds.y + bounds.height / 2);

  await expect(page.getByRole("dialog", { name: imagePath }).getByRole("img", { name: "shot.png" })).toBeVisible();
  await expect(page.getByRole("dialog", { name: "Terminal link actions" })).toHaveCount(0);
});

// swipe drags one finger across the screen through real touch events, in small steps as a finger moves.
async function swipe(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  const client = await page.context().newCDPSession(page);
  const steps = 12;
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [from] });
  for (let step = 1; step <= steps; step += 1) {
    const point = { x: from.x + ((to.x - from.x) * step) / steps, y: from.y + ((to.y - from.y) * step) / steps };
    await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [point] });
  }
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await client.detach();
}

// waitForPageFadeToEnd waits for the fade between two pages (pageMotion.ts) to end. A swipe made during the fade does
// not reach the page: the browser draws its pictures of the two pages over it until the fade ends.
async function waitForPageFadeToEnd(page: Page) {
  await expect(page.locator("html")).not.toHaveAttribute("data-page-changing");
}

test("a swipe across a previewed image steps to the next and previous file, and a swipe down closes it", async ({ page, installation }) => {
  const first = join(installation.home, "first.png");
  const second = join(installation.home, "second.png");
  test.skip(!first.startsWith("/"), "terminal paths are detected only when they start with /");
  await writeFile(first, samplePNG());
  await writeFile(second, samplePNG());
  await openShellPrinting(page, installation, `Saved \x1b[36m${first}\x1b[0m\r\nSaved \x1b[36m${second}\x1b[0m\r\n$ `);
  const bounds = await printedBounds(page, "first.png");
  await page.touchscreen.tap(bounds.x + bounds.width - 4, bounds.y + bounds.height / 2);

  const firstPreview = page.getByRole("dialog", { name: first });
  const firstImage = firstPreview.getByRole("img", { name: "first.png" });
  await expect(firstImage).toBeVisible();
  await expect(firstPreview).toContainText("1 / 2");
  const picture = await firstImage.locator("xpath=..").boundingBox();
  if (picture === null) throw new Error("the preview shows no picture");
  const middle = { x: picture.x + picture.width / 2, y: picture.y + picture.height / 2 };

  await swipe(page, { x: middle.x + 100, y: middle.y }, { x: middle.x - 100, y: middle.y + 10 });
  const secondPreview = page.getByRole("dialog", { name: second });
  await expect(secondPreview.getByRole("img", { name: "second.png" })).toBeVisible();
  await expect(secondPreview).toContainText("2 / 2");
  // The navigation behind the preview stays where it was.
  await expect(page.locator("[data-app-navigation]")).toHaveAttribute("data-navigation-open", "false");

  await swipe(page, { x: middle.x - 100, y: middle.y }, { x: middle.x + 100, y: middle.y });
  await expect(firstImage).toBeVisible();

  await swipe(page, { x: middle.x, y: middle.y - 80 }, { x: middle.x + 10, y: middle.y + 80 });
  await expect(firstPreview).toBeHidden();
});

// A full-screen program draws on the alternate screen, which has no scrollback, and scrolls its own text.
// Claude Code also asks for mouse reports (1000, 1002, 1003 with the SGR encoding 1006); less does not.
const longOutput = Array.from({ length: 80 }, (_, index) => `line ${index + 1}`).join("\r\n");
// A program told of the wheel moves about three lines for each step, so a finger turns one step for every three rows
// it moves; the arrow keys sent to less move a line each, one for every row (touchScrollTarget.ts).
const fullScreenPrograms = [
  { name: "asks for mouse reports, as Claude Code does", modes: "\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h", forward: /\x1b\[<65;\d+;\d+M/g, back: /\x1b\[<64;\d+;\d+M/g, rowsPerStep: 3 },
  { name: "does not ask for mouse reports, as less does", modes: "\x1b[?1049h", forward: /\x1b\[B/g, back: /\x1b\[A/g, rowsPerStep: 1 },
];
function countSent(messages: string[], sequence: RegExp): number {
  return messages.reduce((count, message) => count + (message.match(sequence)?.length ?? 0), 0);
}
for (const program of fullScreenPrograms) {
  test(`a swipe reaches a full-screen program that ${program.name}`, async ({ page, installation }) => {
    // The program starts once it knows the terminal's size, so it draws (and sets its modes) on each resize.
    const messages = await mockTerminalSession(page, `${longOutput}\r\n`, `${program.modes}\x1b[2J\x1b[HFull-screen program`);
    // A flick coasts on after the finger lifts. With reduced motion it does not, so each swipe sends only its own turns.
    await page.emulateMedia({ reducedMotion: "reduce" });
    await openApplication(page, installation);
    await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Terminal", exact: true }).tap();
    const screen = page.getByRole("region", { name: "Terminal for Demo shell" });
    await expect(screen).toContainText("Full-screen program");
    const box = await screen.boundingBox();
    if (box === null) throw new Error("the terminal has no box");
    const middle = box.x + box.width / 2;

    const rowHeight = await drawnRowsIn(screen).first().evaluate((row) => row.getBoundingClientRect().height);
    const steps = Math.round(box.height * 0.4 / rowHeight / program.rowsPerStep);

    // A finger moving up moves the text forward, as the wheel turned towards the person does.
    await swipe(page, { x: middle, y: box.y + box.height * 0.7 }, { x: middle, y: box.y + box.height * 0.3 });
    await expect.poll(() => countSent(messages, program.forward)).toBeGreaterThanOrEqual(steps - 1);
    expect(countSent(messages, program.forward)).toBeLessThanOrEqual(steps + 1);
    expect(countSent(messages, program.back)).toBe(0);

    messages.length = 0;
    await swipe(page, { x: middle, y: box.y + box.height * 0.3 }, { x: middle, y: box.y + box.height * 0.7 });
    await expect.poll(() => countSent(messages, program.back)).toBeGreaterThanOrEqual(steps - 1);
    expect(countSent(messages, program.back)).toBeLessThanOrEqual(steps + 1);
    expect(countSent(messages, program.forward)).toBe(0);
  });
}

test("a swipe across the page opens the navigation and a swipe back closes it", async ({ page, installation }) => {
  await openApplication(page, installation);
  // Closed, the navigation is hidden from the accessibility tree, so it is found by its marker.
  const navigation = page.locator("[data-app-navigation]");
  await expect(navigation).toHaveAttribute("data-navigation-open", "false");

  // Up and down is the page's scroll, not the navigation.
  await swipe(page, { x: 200, y: 520 }, { x: 215, y: 220 });
  await expect(navigation).toHaveAttribute("data-navigation-open", "false");

  await swipe(page, { x: 120, y: 360 }, { x: 330, y: 365 });
  await expect(navigation).toHaveAttribute("data-navigation-open", "true");
  await expect(navigation).toBeInViewport();
  await capture(page, "mobile-navigation-swiped-open");

  await swipe(page, { x: 260, y: 360 }, { x: 40, y: 360 });
  await expect(navigation).toHaveAttribute("data-navigation-open", "false");
  await expect(navigation).not.toBeInViewport();
});

test("a sideways swipe over the terminal opens the navigation, and one up and down scrolls the terminal", async ({ page, installation }) => {
  await mockTerminalSession(page, longOutput);
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Quick navigation" }).getByRole("link", { name: "Terminal", exact: true }).tap();
  const rows = drawnRows(page).filter({ visible: true });
  await expect(rows).toContainText("line 80");
  const box = (await rows.boundingBox())!;
  const navigation = page.locator("[data-app-navigation]");

  await waitForPageFadeToEnd(page);
  await swipe(page, { x: 120, y: box.y + box.height * 0.3 }, { x: 125, y: box.y + box.height * 0.8 });
  await expect(rows).not.toContainText("line 80");
  await expect(navigation).toHaveAttribute("data-navigation-open", "false");

  await swipe(page, { x: 80, y: box.y + box.height / 2 }, { x: 330, y: box.y + box.height / 2 + 6 });
  await expect(navigation).toHaveAttribute("data-navigation-open", "true");
  await expect(navigation).toBeInViewport();
});

// placedByHand reads whether a swipe left the navigation's position written on it rather than in the
// stylesheet, where a later open or close could not move it.
function placedByHand(page: Page) {
  return page.locator("[data-app-navigation]").evaluate((panel: HTMLElement) => panel.style.transform !== "" || panel.style.translate !== "");
}

test("a swipe that pulls the navigation all the way out still lets a tap outside close it", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.locator("[data-app-navigation]");

  // Further than the panel is wide, so it is already where it settles when the finger lifts.
  await swipe(page, { x: 30, y: 360 }, { x: 385, y: 360 });
  await expect(navigation).toHaveAttribute("data-navigation-open", "true");
  await expect.poll(() => placedByHand(page)).toBe(false);

  await page.locator("[data-navigation-backdrop]").click({ position: { x: 370, y: 360 } });
  await expect(navigation).toHaveAttribute("data-navigation-open", "false");
  await expect(navigation).not.toBeInViewport();
});

test("a second finger during a swipe does not leave the navigation half out", async ({ page, installation }) => {
  await openApplication(page, installation);
  const navigation = page.locator("[data-app-navigation]");
  const client = await page.context().newCDPSession(page);

  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: 60, y: 360 }] });
  for (const x of [80, 110, 140, 170]) {
    await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y: 360 }] });
  }
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: 170, y: 360 }, { x: 300, y: 500 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: 200, y: 360 }, { x: 300, y: 500 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [{ x: 200, y: 360 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await client.detach();

  await expect.poll(() => placedByHand(page)).toBe(false);
  const open = await navigation.getAttribute("data-navigation-open");
  if (open === "true") await expect(navigation).toBeInViewport({ ratio: 1 });
  else await expect(navigation).not.toBeInViewport();
});

test.describe("desktop pointer compatibility", () => {
  test.use({ viewport: { width: 1440, height: 900 }, hasTouch: false, isMobile: false });
  test("a mouse click selects a directory and a double click opens it", async ({ page, installation }) => {
    const requested = await mockFiles(page);
    await openApplication(page, installation);
    await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "SFTP", exact: true }).click();
    await page.getByRole("button", { name: "Host", exact: true }).click();
    await page.getByRole("dialog").getByText("bastion", { exact: true }).click();
    const folder = page.getByRole("button", { name: "projects", exact: true });
    await expect(folder).toBeVisible();
    await folder.click();
    await expect(page.getByRole("checkbox", { name: "Select projects", exact: true })).toBeChecked();
    expect(requested).toEqual(["/workspace"]);
    await folder.dblclick();
    await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace/projects");
    expect(requested).toEqual(["/workspace", "/workspace/projects"]);
  });
});

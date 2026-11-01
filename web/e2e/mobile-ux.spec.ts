import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openApplication, test, type Installation } from "./support/environment";
import { samplePNG } from "./support/images";
import { drawnRows, drawnRowsIn, drawnSpan } from "./support/terminal";

// All screenshots are opt-in and contain only the isolated installation's
// example hosts and synthetic SFTP/terminal data. Never capture real servers.
async function capture(page: Page, name: string) {
  const directory = process.env.SSHC_MOBILE_VISUAL_DIR;
  if (directory === undefined) return;
  await mkdir(directory, { recursive: true });
  await page.screenshot({ path: join(directory, `${name}.png`), animations: "disabled" });
}

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

test("one tap opens folders with immediate loading feedback and checkboxes enter selection mode", async ({ page, installation }) => {
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
  await page.getByRole("checkbox", { name: "Select projects", exact: true }).check();
  await page.getByRole("button", { name: "archives", exact: true }).tap();
  await expect(page.getByRole("checkbox", { name: "Select archives", exact: true })).toBeChecked();
  expect(requested).toHaveLength(callsBeforeSelect);
  await expect(page.getByRole("button", { name: "Actions for 2 selected items", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Clear selection", exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
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

test("coming back to the app after typing leaves the keyboard down", async ({ page, installation }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openShellFromNavigation(page);
  await expect(drawnRows(page).filter({ visible: true })).toContainText(/[$#%>]/, { timeout: 20_000 });
  const screenBox = (await drawnRows(page).filter({ visible: true }).boundingBox())!;
  await page.touchscreen.tap(screenBox.x + screenBox.width / 2, screenBox.y + screenBox.height / 2);
  const textBox = page.getByLabel("Text to send to the terminal", { exact: true });
  await expect(textBox).toBeFocused();

  // Coming back to the page hides the app while the vault is rechecked; the field that had the focus is not given
  // it back, which on a phone would raise the keyboard (Android keeps the focus in the field when its keyboard is
  // closed with Back).
  const app = page.locator(".sshc-app > .contents");
  // On a phone the recheck takes long enough for the browser to move the focus off the hidden field. Answered at
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

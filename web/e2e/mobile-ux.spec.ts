import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openApplication, test } from "./support/environment";

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
  largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20,
  jobs: [{
    id: "mobile-demo-transfer", batchId: "mobile-demo-batch", batchName: "Demo download", batchKind: "file",
    alias: "bastion", sourceAlias: "", sourcePath: "", operation: "", direction: "download", kind: "file",
    name: "reports.zip", remotePath: "/workspace/reports.zip", totalBytes: 10 << 20, transferredBytes: 4 << 20,
    bytesPerSecond: 0, remainingSeconds: -1, status: "paused", allowedActions: ["resume", "cancel"],
    attempt: 1, problem: "", lastModified: 0, expectedRevision: "", sourceFingerprint: "", overwrite: false,
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
  await page.getByRole("button", { name: "Connect", exact: true }).tap();
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
  await page.getByRole("button", { name: "Back", exact: true }).tap();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
  const callsBeforeSelect = requested.length;
  await page.getByRole("checkbox", { name: "Select projects", exact: true }).check();
  await page.getByRole("button", { name: "archives", exact: true }).tap();
  await expect(page.getByRole("checkbox", { name: "Select archives", exact: true })).toBeChecked();
  expect(requested).toHaveLength(callsBeforeSelect);
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/workspace");
});

test("quick navigation and terminal keys remain available in portrait and touch landscape", async ({ page, installation }) => {
  const messages: string[] = [];
  await page.route("**/api/v1/terminal/sessions**", (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) {
      return route.fulfill({ status: 201, json: { streamTicket: "mobile-demo-ticket" } });
    }
    return route.fulfill({ json: { sessions: [{ id: "mobile-demo-shell", kind: "shell", title: "Demo shell", startedAt: "2026-09-12T00:00:00Z", state: "connected", problem: "", forwards: [] }], maxSessions: 12 } });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    socket.send(Buffer.from("Mobile terminal demo\r\nFiles: projects/  archives/  report.txt\r\n$ "));
    socket.onMessage((data) => messages.push(typeof data === "string" ? data : data.toString("utf8")));
  });
  await page.addInitScript(() => window.sessionStorage.setItem("sshc.terminal.live-workspace.v1", JSON.stringify({
    version: 1, root: { pane: { id: "mobile-demo-pane", sessionId: "mobile-demo-shell" } }, focusedPaneId: "mobile-demo-pane", focusModePaneId: null,
  })));
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
  await expect(page.getByRole("region", { name: "Console for Demo shell" })).toContainText("Mobile terminal demo");
  await capture(page, "mobile-terminal-390x640");
  for (const viewport of [{ width: 390, height: 640 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport);
    await expect(keys).toBeVisible();
    await expectInside(page, keys);
    await expectInside(page, navigation);
    await keys.getByRole("button", { name: "↑", exact: true }).tap();
    await expect.poll(() => messages.some((message) => message.includes("\u001b[A"))).toBe(true);
    messages.length = 0;
  }
  await capture(page, "mobile-terminal-844x390");
  await navigation.getByRole("link", { name: "Home", exact: true }).tap();
  await expect(page).toHaveURL(new URL("/", installation.url).toString());
  await expect(navigation.getByRole("link", { name: "Home", exact: true })).toHaveAttribute("aria-current", "page");
});

test.describe("desktop pointer compatibility", () => {
  test.use({ viewport: { width: 1440, height: 900 }, hasTouch: false, isMobile: false });
  test("a mouse click selects a directory and a double click opens it", async ({ page, installation }) => {
    const requested = await mockFiles(page);
    await openApplication(page, installation);
    await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "SFTP", exact: true }).click();
    await page.getByRole("button", { name: "Host", exact: true }).click();
    await page.getByRole("dialog").getByText("bastion", { exact: true }).click();
    await page.getByRole("button", { name: "Connect", exact: true }).click();
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

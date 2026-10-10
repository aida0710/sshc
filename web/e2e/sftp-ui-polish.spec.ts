import { join } from "node:path";
import { mkdir } from "node:fs/promises";
import type { Locator, Page } from "@playwright/test";
import type { TransferJob, TransferJobList } from "../src/sftp/api";
import { expect, openApplication, openSection, test } from "./support/environment";
import { openLocalSFTPDirectory } from "./support/sftp";

// Phones keep touch controls when rotated beyond the portrait breakpoint.
test.use({ hasTouch: true });

function transferJob(id: string, overrides: Partial<TransferJob>): TransferJob {
  return {
    id: `fixture_${id}`, batchId: `batch-${id}`, batchName: `${id}.txt`, batchKind: "file", alias: "bastion",
    sourceAlias: "nas", sourcePath: `/source/${id}.txt`, operation: "copy", direction: "remote", kind: "file",
    name: `${id}.txt`, remotePath: `/destination/${id}.txt`, totalBytes: 4096, transferredBytes: 1024,
    bytesPerSecond: 0, remainingSeconds: -1, status: "paused", allowedActions: ["resume", "cancel"],
    attempt: 1, reconnectAttempt: 0, reconnectAt: "", problem: "", lastModified: 0,
    expectedRevision: "", sourceFingerprint: "", overwrite: false, downloadRevision: "", downloadParts: [],
    createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", ...overrides,
  };
}

test("keeps transfer states visible while settings are folded and identifies both endpoints", async ({ page, installation }) => {
  const listing: TransferJobList = {
    maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: true,
    largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20,
    speedLimitBytesPerSecond: 0, autoReconnect: true, maxReconnectAttempts: 3,
    jobs: [
      transferJob("copy", { status: "running", bytesPerSecond: 1024, allowedActions: ["pause", "cancel"] }),
      transferJob("get", { operation: "get", sourceAlias: "bastion", sourcePath: "/srv/get.txt", remotePath: "/home/engine/get.txt" }),
      transferJob("put", { operation: "put", sourcePath: "/home/engine/put.txt", remotePath: "/srv/put.txt", status: "needs_overwrite" }),
      transferJob("reconnect", { status: "reconnecting", reconnectAttempt: 2 }),
      transferJob("failed", { status: "failed", problem: "sftp_connection_lost", allowedActions: ["retry", "remove"] }),
    ],
  };
  await page.route("**/api/v1/sftp/transfers", (route) => route.fulfill({ json: listing }));
  await page.setViewportSize({ width: 1280, height: 720 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const manager = page.getByRole("region", { name: "Transfer Manager" });
  for (const label of ["1 transferring", "1 paused", "1 reconnecting", "Needs attention: 1", "1 failed"]) {
    await expect(manager.getByText(label, { exact: true })).toBeVisible();
  }
  await manager.getByRole("button", { name: "Expand Transfer Manager", exact: true }).click();
  const speed = manager.getByRole("spinbutton", { name: "Speed limit", exact: true });
  await expect(speed).toBeHidden();
  await manager.getByRole("button", { name: "Show details for get.txt", exact: true }).click();
  await expect(manager.getByText("bastion:/srv/get.txt → Local:/home/engine/get.txt", { exact: true })).toBeVisible();
  await manager.getByRole("button", { name: "Show details for put.txt", exact: true }).click();
  await expect(manager.getByText("Local:/home/engine/put.txt → bastion:/srv/put.txt", { exact: true })).toBeVisible();
  const settings = manager.getByRole("button", { name: "Transfer settings", exact: true });
  await settings.focus();
  await settings.press("Enter");
  const settingsDialog = page.getByRole("dialog", { name: "Transfer settings", exact: true });
  await expect(settingsDialog.getByRole("spinbutton", { name: "Speed limit", exact: true })).toBeVisible();
  await settingsDialog.getByRole("button", { name: "Close transfer settings", exact: true }).click();
  await expect(settingsDialog).toBeHidden();
  await expect(settings).toBeFocused();
  await expect(manager.getByText("Needs attention: 1", { exact: true })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "transfer-states-and-routes-en.png"), animations: "disabled" });
});

// Compact navigation and file controls must remain easy to tap on a phone.
const minimumTouchTargetSize = 44;

async function expectTouchTarget(target: Locator): Promise<void> {
  await expect(target).toBeVisible();
  // A viewport resize can return before React replaces the desktop controls.
  await expect.poll(async () => {
    const bounds = await target.boundingBox();
    return Math.min(bounds?.width ?? 0, bounds?.height ?? 0);
  }).toBeGreaterThanOrEqual(minimumTouchTargetSize);
}

async function expectCompactControls(page: Page): Promise<void> {
  const strip = page.getByRole("tablist", { name: "File tabs", exact: true });
  await expect(strip).toHaveCount(1);
  await expect(page.getByRole("navigation", { name: "SFTP pane switcher", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Open right pane", exact: true })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: /^(Left|Right):/ })).toHaveCount(0);
  for (const target of await strip.getByRole("tab").all()) {
    await target.locator("..").scrollIntoViewIfNeeded();
    await expectTouchTarget(target);
  }
  for (const control of await strip.getByRole("button").all()) await expectTouchTarget(control);
  // Measurements may scroll an inactive tab into view. Restore the selected
  // tab's whole group so its close control stays reachable in the final view.
  const selectedGroup = strip.getByRole("tab", { selected: true }).locator("..");
  await selectedGroup.scrollIntoViewIfNeeded();
  const selectedClose = selectedGroup.getByRole("button", { name: /^Close the .* tab$/ });
  if (await selectedClose.count() > 0) {
    await expect.poll(async () => {
      const stripBounds = await strip.boundingBox();
      const closeBounds = await selectedClose.boundingBox();
      return stripBounds !== null && closeBounds !== null &&
        closeBounds.x >= stripBounds.x && closeBounds.x + closeBounds.width <= stripBounds.x + stripBounds.width;
    }).toBe(true);
  }
  await expect.poll(async () => {
    const stripBounds = await strip.boundingBox();
    const groupBounds = await selectedGroup.boundingBox();
    return stripBounds !== null && groupBounds !== null &&
      groupBounds.x >= stripBounds.x && groupBounds.x + groupBounds.width <= stripBounds.x + stripBounds.width &&
      groupBounds.y >= stripBounds.y && groupBounds.y + groupBounds.height <= stripBounds.y + stripBounds.height;
  }).toBe(true);
  await expectTouchTarget(page.getByRole("button", { name: "New tab", exact: true }));
  const pane = page.getByRole("tabpanel");
  await expect(pane).toHaveCount(1);
  await expectTouchTarget(page.getByRole("button", { name: "Host", exact: true }));
  await expectTouchTarget(pane.getByRole("button", { name: "Search files", exact: true }));
  const clearSelection = pane.getByRole("button", { name: "Clear selection", exact: true });
  if (await clearSelection.isVisible()) {
    await expectTouchTarget(clearSelection);
    await expectTouchTarget(pane.getByRole("button", { name: /^Actions for / }));
  } else {
    for (const name of ["Back", "Folder actions"]) {
      await expectTouchTarget(pane.getByRole("button", { name, exact: true }));
    }
    await expectTouchTarget(pane.getByTestId("sftp-current-path"));
  }
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
}

async function expectFileRowInsideList(pane: Locator, name: string): Promise<void> {
  const list = pane.getByTestId("sftp-file-list");
  const row = list.getByRole("button", { name, exact: true });
  await row.scrollIntoViewIfNeeded();
  // Visibility alone allows a row to sit beneath the clipped list or navigation.
  await expect.poll(async () => {
    const listBounds = await list.boundingBox();
    const rowBounds = await row.boundingBox();
    return listBounds !== null && rowBounds !== null &&
      rowBounds.height >= minimumTouchTargetSize &&
      rowBounds.y >= listBounds.y - 1 && rowBounds.y + rowBounds.height <= listBounds.y + listBounds.height + 1 &&
      rowBounds.x >= listBounds.x - 1 && rowBounds.x + rowBounds.width <= listBounds.x + listBounds.width + 1;
  }).toBe(true);
}

async function captureCompactFiles(page: Page, name: string): Promise<void> {
  const directory = process.env.SSHC_VISUAL_DIR;
  if (directory === undefined) return;
  await mkdir(directory, { recursive: true });
  await page.screenshot({ path: join(directory, `${name}.png`), animations: "disabled" });
}

test("adds a compact file tab and preserves both directories and selections without choosing a side", async ({ page, installation }) => {
  const firstPath = join(installation.home, ".ssh/mobile-first");
  const secondPath = join(installation.home, ".ssh/mobile-second");
  await installation.write("mobile-first/notes.txt", "notes");
  await installation.write("mobile-second/report.txt", "report");
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: firstPath });
  await page.getByRole("checkbox", { name: "Select notes.txt", exact: true }).check();
  await page.setViewportSize({ width: 390, height: 640 });
  await expectCompactControls(page);
  await page.getByRole("button", { name: "New tab", exact: true }).click();
  await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel"), directory: secondPath });
  const strip = page.getByRole("tablist", { name: "File tabs", exact: true });
  await expect(strip.getByRole("tab")).toHaveCount(2);
  const firstTab = strip.getByRole("tab", { name: "Local:mobile-first", exact: true });
  const secondTab = strip.getByRole("tab", { name: "Local:mobile-second", exact: true });
  await expect(secondTab).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("button", { name: "Host", exact: true })).toHaveAttribute("data-value", "sshc://local");
  await page.getByRole("checkbox", { name: "Select report.txt", exact: true }).check();
  await expectCompactControls(page);
  await firstTab.click();
  const pane = page.getByRole("tabpanel");
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", firstPath);
  await expect(page.getByRole("button", { name: "Host", exact: true })).toHaveAttribute("data-value", "sshc://local");
  await expect(pane.getByRole("checkbox", { name: "Select notes.txt", exact: true })).toBeChecked();
  await secondTab.click();
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", secondPath);
  await expect(pane.getByRole("checkbox", { name: "Select report.txt", exact: true })).toBeChecked();
  await expectCompactControls(page);
  await expectFileRowInsideList(pane, "report.txt");
  await captureCompactFiles(page, "compact-sftp-local-tabs-390x640-en");
  await page.setViewportSize({ width: 844, height: 390 });
  await expectCompactControls(page);
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", secondPath);
  await expect(pane.getByRole("checkbox", { name: "Select report.txt", exact: true })).toBeChecked();
  await expectFileRowInsideList(pane, "report.txt");
});

test("restores two compact sources in one tab strip and preserves the host, path and selection", async ({ page, installation }) => {
  await page.addInitScript(() => {
    window.localStorage.setItem("sshc.sftp.panes.v1", JSON.stringify([
      { tabs: [{ alias: "sshc://local", path: "/home/engine" }], activeIndex: 0 },
      { tabs: [{ alias: "bastion", path: "/srv" }], activeIndex: 0 },
    ]));
  });
  const entry = (name: string, parent: string) => ({
    name, path: `${parent}/${name}`, type: "file", size: 4096, mode: "0644",
    modifiedAt: "2026-10-10T00:00:00Z", revision: `${name}-v1`,
  });
  await page.route("**/api/v1/sftp/local/entries**", (route) => route.fulfill({ json: {
    path: "/home/engine", home: "/home/engine", entries: [entry("notes.txt", "/home/engine")],
  } }));
  let remoteReads = 0;
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => {
    remoteReads += 1;
    return route.fulfill({ json: { path: "/srv", entries: [entry("report.txt", "/srv")] } });
  });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.setViewportSize({ width: 390, height: 640 });
  const strip = page.getByRole("tablist", { name: "File tabs", exact: true });
  await expect(strip).toHaveCount(1);
  await expect(strip.getByRole("tab")).toHaveCount(2);
  const localTab = strip.getByRole("tab", { name: "Local:engine", exact: true });
  const remoteTab = strip.getByRole("tab", { name: "bastion:srv", exact: true });
  const pane = page.getByRole("tabpanel");
  await expect(pane.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
  await pane.getByRole("checkbox", { name: "Select notes.txt", exact: true }).check();
  await expectCompactControls(page);
  await remoteTab.click();
  await expect(page.getByRole("button", { name: "Host", exact: true })).toHaveAttribute("data-value", "bastion");
  await pane.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv");
  await pane.getByRole("checkbox", { name: "Select report.txt", exact: true }).check();
  await expectCompactControls(page);
  await localTab.click();
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/engine");
  await expect(page.getByRole("button", { name: "Host", exact: true })).toHaveAttribute("data-value", "sshc://local");
  await expect(pane.getByRole("checkbox", { name: "Select notes.txt", exact: true })).toBeChecked();
  await remoteTab.click();
  await expect(page.getByRole("button", { name: "Host", exact: true })).toHaveAttribute("data-value", "bastion");
  await expect(pane.getByRole("checkbox", { name: "Select report.txt", exact: true })).toBeChecked();
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv");
  expect(remoteReads).toBe(1);
  expect(await page.evaluate(() => JSON.parse(localStorage.getItem("sshc.sftp.panes.v1") ?? "[]"))).toHaveLength(2);
  await expectCompactControls(page);
  await expectFileRowInsideList(pane, "report.txt");
  await captureCompactFiles(page, "compact-sftp-restored-tabs-390x640-en");
  await page.setViewportSize({ width: 844, height: 390 });
  await expectCompactControls(page);
  await expect(pane.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv");
  await expect(pane.getByRole("checkbox", { name: "Select report.txt", exact: true })).toBeChecked();
  await expectFileRowInsideList(pane, "report.txt");
});

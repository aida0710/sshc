import { join } from "node:path";
import type { TransferJob, TransferJobList } from "../src/sftp/api";
import { expect, openApplication, openSection, test } from "./support/environment";

function transferJob(id: string, overrides: Partial<TransferJob>): TransferJob {
  return {
    id, batchId: `batch-${id}`, batchName: `${id}.txt`, batchKind: "file", alias: "bastion",
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
  await expect(manager.getByText("bastion:/srv/get.txt → Local:/home/engine/get.txt", { exact: true })).toBeVisible();
  await expect(manager.getByText("Local:/home/engine/put.txt → bastion:/srv/put.txt", { exact: true })).toBeVisible();
  const settings = manager.getByText("Transfer settings", { exact: true });
  await settings.focus();
  await settings.press("Enter");
  await expect(speed).toBeVisible();
  await settings.press("Enter");
  await expect(speed).toBeHidden();
  await expect(manager.getByText("Needs attention: 1", { exact: true })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "transfer-states-and-routes-en.png"), animations: "disabled" });
});

test("switches compact SFTP panes while preserving the host, path and selection", async ({ page, installation }) => {
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
  await page.setViewportSize({ width: 390, height: 640 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const switcher = page.getByRole("navigation", { name: "SFTP pane switcher" });
  await expect(page.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
  await switcher.getByRole("button", { name: "Right: bastion", exact: true }).click();
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv");
  await page.getByRole("checkbox", { name: "Select report.txt", exact: true }).check();
  await switcher.getByRole("button", { name: "Left: Local", exact: true }).click();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/engine");
  await expect(page.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();
  await switcher.getByRole("button", { name: "Right: bastion", exact: true }).click();
  await expect(page.getByRole("checkbox", { name: "Select report.txt", exact: true })).toBeChecked();
  await expect(page.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv");
  expect(remoteReads).toBe(1);
  expect(await page.evaluate(() => JSON.parse(localStorage.getItem("sshc.sftp.panes.v1") ?? "[]"))).toHaveLength(2);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "compact-sftp-pane-switcher-en.png"), animations: "disabled" });
});

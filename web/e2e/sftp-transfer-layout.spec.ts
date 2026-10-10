import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";
import type { TransferJob, TransferJobList } from "../src/sftp/api";

const listing: TransferJobList = {
  maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: true,
  largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20,
  speedLimitBytesPerSecond: 2048 * 1024, autoReconnect: true, maxReconnectAttempts: 3,
  excludePatterns: [".git", "node_modules", "*.log"], jobs: [],
};

function transferJob(name: string, status: TransferJob["status"]): TransferJob {
  const id = name.replaceAll(".", "_");
  return {
    id: `fixture_${id}`, batchId: `batch-${id}`, batchName: name, batchKind: "file", alias: "bastion",
    sourceAlias: "nas", sourcePath: `/backups/${name}`, operation: "copy", direction: "remote", kind: "file",
    name, remotePath: `/srv/releases/${name}`, totalBytes: 64 << 20, transferredBytes: 24 << 20,
    bytesPerSecond: status === "running" ? 4 << 20 : 0, remainingSeconds: status === "running" ? 10 : -1,
    status, allowedActions: status === "paused" ? ["resume", "cancel"] : ["pause", "cancel"],
    attempt: 1, reconnectAttempt: status === "reconnecting" ? 2 : 0, reconnectAt: "", problem: "",
    lastModified: 0, expectedRevision: "", sourceFingerprint: "", overwrite: false,
    downloadRevision: "", downloadParts: [], excludePatterns: [],
    createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z",
  };
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 640 }, { width: 844, height: 390 }]) {
  const mobile = viewport.width < 1024;
  test(`keeps compact transfer rows and separate settings at ${viewport.width}x${viewport.height}`, async ({ page, installation }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.addInitScript(() => {
      localStorage.setItem("sshc.sftp.queueView", JSON.stringify({ collapsed: true, height: 224 }));
    });
    await page.route("**/api/v1/sftp/transfers", route => route.fulfill({ json: {
      ...listing, jobs: [transferJob("app-release.tar.gz", "running"), transferJob("backup.sql", "paused"), transferJob("access-archive.zip", "reconnecting")],
    } }));
    await openApplication(page, installation);
    await openSection(page, "SFTP");
    await page.setViewportSize(viewport);
    const dock = page.getByRole("button", { name: "Expand Transfer Manager", exact: true });
    if (mobile) {
      await expect(dock).toHaveAttribute("aria-haspopup", "dialog");
      expect((await dock.boundingBox())!.height).toBeLessThanOrEqual(48);
    }
    await dock.click();
    const manager = mobile ? page.getByRole("dialog", { name: "Transfer Manager", exact: true }) : page.getByRole("region", { name: "Transfer Manager", exact: true });
    const card = manager.getByRole("region", { name: "app-release.tar.gz", exact: true });
    await expect(card.getByText("app-release.tar.gz", { exact: true })).toHaveCount(1);
    await expect(card.getByText("nas → bastion", { exact: true })).toBeVisible();
    expect((await card.boundingBox())!.height).toBeLessThanOrEqual(mobile ? 140 : 80);
    const details = card.getByRole("button", { name: "Show details for app-release.tar.gz", exact: true });
    if (mobile) {
      for (const button of await card.getByRole("button").all()) {
        const bounds = (await button.boundingBox())!;
        expect(Math.min(bounds.width, bounds.height)).toBeGreaterThanOrEqual(44);
      }
    }
    await details.click();
    await expect(card.getByText("nas:/backups/app-release.tar.gz → bastion:/srv/releases/app-release.tar.gz", { exact: true })).toBeVisible();
    await card.getByRole("button", { name: "Hide details for app-release.tar.gz", exact: true }).click();
    const settings = manager.getByRole("button", { name: "Transfer settings", exact: true });
    await settings.click();
    const settingsDialog = page.getByRole("dialog", { name: "Transfer settings", exact: true });
    await expect(settingsDialog.getByRole("group", { name: "Speed and recovery", exact: true })).toBeVisible();
    await expect(settingsDialog.getByRole("spinbutton", { name: "Speed limit", exact: true })).toHaveValue("2048");
    if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, `transfer-settings-${viewport.width}x${viewport.height}-en.png`), animations: "disabled" });
    await settingsDialog.getByRole("button", { name: "Close transfer settings", exact: true }).click();
    await expect(settings).toBeFocused();
    await expect(manager).toBeVisible();
    if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, `transfer-layout-${viewport.width}x${viewport.height}-en.png`), animations: "disabled" });
  });
}

// A phone keeps its touch targets when rotated beyond the portrait breakpoint.
test.use({ hasTouch: true });

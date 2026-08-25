import { expect, openApplication, openSection, test } from "./support/environment";

test("shows push, preview, apply, persisted success, and a later failure as distinct results", async ({
  page,
  installation,
}) => {
  const summary = {
    createdAt: "2026-08-12T01:30:00Z",
    fileCount: 12,
    sourceBytes: 4_800_000,
    snapshotBytes: 1_900_000,
  };
  let lastOperation: Record<string, unknown> | undefined;
  let refusePush = false;
  const status = () => ({
    configured: true,
    keyConfigured: true,
    locked: false,
    auto: { enabled: false, phase: "idle" },
    endpoint: "https://s3.example.invalid",
    bucket: "sshc",
    path: "",
    region: "auto",
    synced: lastOperation !== undefined,
    direction: "both",
    lastSyncedAt: lastOperation === undefined ? undefined : summary.createdAt,
    origin: lastOperation === undefined ? undefined : "origin-e2e",
    fileCount: lastOperation === undefined ? undefined : summary.fileCount,
    lastOperation,
  });

  await page.route("**/api/v1/sync**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/v1/sync" && request.method() === "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(status()) });
      return;
    }
    if (path === "/api/v1/sync/bucket" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          checkedAt: "2026-08-25T02:10:00Z",
          localIsLive: false,
          historyTruncated: false,
          live: {
            key: "workspace.tar.gz.enc",
            size: 1_900_000,
            lastModified: "2026-08-25T02:09:00Z",
          },
          history: [
            {
              key: "snapshots/2026-08-25-020900-4f524947494e3031-aabbccddeeff0011-000001.tar.gz.enc",
              size: 1_900_000,
              lastModified: "2026-08-25T02:09:00Z",
            },
          ],
        }),
      });
      return;
    }
    if (path === "/api/v1/sync/push" && request.method() === "POST") {
      if (refusePush) {
        await route.fulfill({
          status: 409,
          contentType: "application/problem+json",
          body: JSON.stringify({ code: "sync_remote_moved", message: "request rejected" }),
        });
        return;
      }
      const result = { summary, objectCount: 2, uploadedBytes: 3_800_000, completedAt: "2026-08-12T01:30:03Z" };
      lastOperation = { kind: "push", ...result };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: status(), result }),
      });
      return;
    }
    if (path === "/api/v1/sync/pull" && request.method() === "POST") {
      const apply = request.postDataJSON().apply === true;
      const result = {
        applied: apply,
        summary,
        downloadedBytes: 1_900_000,
        completedAt: apply ? "2026-08-12T01:32:00Z" : "2026-08-12T01:31:00Z",
        conflicts: [],
        written: ["config"],
        removed: [],
      };
      if (apply) {
        lastOperation = {
          kind: "apply",
          summary,
          downloadedBytes: result.downloadedBytes,
          written: 1,
          removed: 0,
          completedAt: result.completedAt,
        };
      }
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(result) });
      return;
    }
    await route.continue();
  });

  await openApplication(page, installation);
  await openSection(page, "Sync");
  await expect(page.getByText("Dated history · 1")).toBeVisible();
  await expect(page.getByText("The remote generation differs")).toBeVisible();
  await expect(page.getByText("Legacy snapshot migration")).toHaveCount(0);

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.2-sync-desktop.png`, fullPage: true });
    await page.getByRole("heading", { name: "Bucket status" }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.2-sync-bucket-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 360, height: 800 });
    await page.getByRole("heading", { name: "Remote sync" }).scrollIntoViewIfNeeded();
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.2-sync-mobile.png`, fullPage: true });
    await page.getByRole("heading", { name: "Bucket status" }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.2-sync-bucket-mobile.png`, fullPage: true });
    await page.setViewportSize({ width: 1280, height: 720 });
  }
  await page.getByRole("button", { name: "Push this workspace" }).click();
  await expect(page.getByRole("heading", { name: "This push" })).toBeVisible();
  await expect(page.getByText("S3 transfer 3.8 MB (2 objects, history + live)")).toBeVisible();

  await page.getByRole("button", { name: "Check for changes" }).click();
  await expect(page.getByRole("heading", { name: "Pull preview" })).toBeVisible();
  await expect(page.getByText("Downloaded 1.9 MB · 4.8 MB after opening")).toBeVisible();
  await page.getByRole("button", { name: "Apply the snapshot" }).click();
  await expect(page.getByRole("heading", { name: "Apply result" })).toBeVisible();
  await expect(page.getByText("Downloaded again for apply: 1.9 MB")).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Previous success" })).toBeVisible();
  refusePush = true;
  await page.getByRole("button", { name: "Push this workspace" }).click();
  await expect(page.getByRole("alert")).toContainText("update was cancelled");
  await expect(page.getByRole("alert")).toContainText("dated history copy");
  await expect(page.getByRole("heading", { name: "Previous success" })).toBeVisible();
});

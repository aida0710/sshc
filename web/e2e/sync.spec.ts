import { expect, openApplication, openSection, test } from "./support/environment";

test("checks an existing destination and verifies its shared key before saving", async ({ page, installation }) => {
  let completed: Record<string, unknown> | undefined;
  const unconfigured = {
    configured: false, keyConfigured: false, locked: false,
    auto: { enabled: false, phase: "idle" }, endpoint: "", bucket: "",
    path: "", region: "", synced: false, direction: "both",
  };
  await page.route("**/api/v1/sync**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/v1/sync" && request.method() === "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(unconfigured) });
      return;
    }
    if (path === "/api/v1/sync/setup/check" && request.method() === "POST") {
      await route.fulfill({
        status: 200, contentType: "application/json",
        body: JSON.stringify({ state: "existing", historyPresent: true, checkedAt: "2026-08-26T12:00:00Z", etag: '"head-1"' }),
      });
      return;
    }
    if (path === "/api/v1/sync/setup" && request.method() === "PUT") {
      completed = request.postDataJSON();
      await route.fulfill({
        status: 200, contentType: "application/json",
        body: JSON.stringify({ status: { ...unconfigured, configured: true, keyConfigured: true, endpoint: "https://r2.example.test", bucket: "sshc", path: "team" } }),
      });
      return;
    }
    await route.continue();
  });

  await openApplication(page, installation);
  await openSection(page, "Sync");
  await page.getByLabel("Endpoint").fill("https://r2.example.test");
  await page.getByLabel("Bucket name").fill("sshc");
  await page.getByLabel("Path in the bucket").fill("team");
  await page.getByLabel("Access key ID").fill("AKID");
  await page.getByLabel("Secret access key").fill("secret");
  await page.getByRole("button", { name: "Check connection" }).click();
  await expect(page.getByText("Existing sync data was found.")).toBeVisible();
  await expect(page.getByText(/actual snapshot will be decrypted/i)).toBeVisible();
  await page.getByRole("textbox", { name: "Key", exact: true }).fill("AB12-CD34-EF56-GH78-JK90-MN12");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 360, height: 800 });
    const navigation = page.getByRole("button", { name: "Navigation" });
    if (await navigation.getAttribute("aria-expanded") === "true") await navigation.click();
    await page.waitForTimeout(500);
    await page.getByText("Existing sync data was found.").scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.15.0-sync-setup-existing-mobile.png` });
  }
  await page.getByRole("button", { name: "Verify and save" }).click();
  await expect.poll(() => completed).toMatchObject({
    expectedState: "existing", expectedETag: '"head-1"', historyPresent: true,
    direction: "both", key: "AB12-CD34-EF56-GH78-JK90-MN12",
  });
  await expect(page.getByText("Manage sync settings")).toBeVisible();
});

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
  let pushedMessage = "";
  let localChanges = true;
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
    if (path === "/api/v1/sync/history" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          checkedAt: "2026-08-25T02:10:00Z",
          headRevision: "b".repeat(64),
          historyTruncated: false,
          downloadTruncated: false,
          downloadedBytes: 5_700_000,
          skipped: 0,
          revisions: [
            {
              key: "snapshots/2026-08-25-020900-head.tar.gz.enc",
              revision: "b".repeat(64),
              parentRevision: "a".repeat(64),
              message: "Update config and production hosts",
              createdAt: "2026-08-25T02:09:00Z",
              origin: "device-head",
              fileCount: 12,
              size: 1_900_000,
              lastModified: "2026-08-25T02:09:00Z",
              relation: "head",
            },
            {
              key: "snapshots/2026-08-24-180000-parent.tar.gz.enc",
              revision: "a".repeat(64),
              message: "Add initial SSH workspace",
              createdAt: "2026-08-24T18:00:00Z",
              origin: "device-old",
              fileCount: 11,
              size: 1_800_000,
              lastModified: "2026-08-24T18:00:00Z",
              relation: "ancestor",
            },
          ],
        }),
      });
      return;
    }
    if (path === "/api/v1/sync/history/diff" && request.method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          fromRevision: "a".repeat(64),
          toRevision: "b".repeat(64),
          added: ["connections/production.conf"],
          modified: ["config"],
          removed: ["connections/retired.conf"],
          downloadedBytes: 3_700_000,
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
      pushedMessage = request.postDataJSON().message ?? "";
      localChanges = false;
      const result = { summary, objectCount: 2, uploadedBytes: 3_800_000, completedAt: "2026-08-12T01:30:03Z" };
      lastOperation = { kind: "push", ...result };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: status(), result }),
      });
      return;
    }
    if (path === "/api/v1/sync/push" && request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          localChanges
            ? { message: "Update config", added: 1, modified: 1, removed: 0 }
            : { message: "Record current workspace", added: 0, modified: 0, removed: 0 },
        ),
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
        remoteETag: '"sync-e2e-generation"',
        remoteRevision: "b".repeat(64),
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
  await expect(page.getByRole("button", { name: "Sync now" })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Receive from remote" })).toBeVisible();
  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-sync-settings-collapsed.png`, fullPage: true });
  }
  await page.getByText("Manage sync settings").click();
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-sync-settings-expanded.png`, fullPage: true });
  }
  await expect(page.getByText("Dated history · 1")).toBeVisible();
  await expect(page.getByText("The remote generation differs")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Encrypted revision history" })).toBeVisible();
  await expect(page.getByLabel("Commit message")).toHaveValue("Update config");
  await page.getByRole("button", { name: /aaaaaaaaaaaa.*ancestor/i }).click();
  await expect(page.getByText("Modified · 1")).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.15.0-sync-desktop.png`, fullPage: true });
    await page.getByRole("heading", { name: "Bucket status" }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.15.0-sync-history-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 360, height: 800 });
    await page.getByRole("heading", { name: "Remote sync" }).scrollIntoViewIfNeeded();
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.15.0-sync-mobile.png`, fullPage: true });
    await page.getByRole("heading", { name: "Encrypted revision history" }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.15.0-sync-history-mobile.png`, fullPage: true });
    await page.setViewportSize({ width: 1280, height: 720 });
  }
  await page.getByRole("button", { name: "Push this workspace" }).click();
  expect(pushedMessage).toBe("Update config");
  await expect(page.getByRole("heading", { name: "This push" })).toBeVisible();
  await expect(page.getByText("S3 transfer 3.8 MB (2 objects, history + live)")).toBeVisible();
  await expect(page.getByText("There are no local changes to push.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Push this workspace" })).toBeDisabled();
  if (visualDirectory !== undefined) {
    await page.getByLabel("Commit message").scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.15.0-sync-no-changes-desktop.png`, fullPage: true });
  }

  await page.getByRole("button", { name: "Check for changes" }).click();
  await expect(page.getByRole("heading", { name: "Pull preview" })).toBeVisible();
  await expect(page.getByText("Downloaded 1.9 MB · 4.8 MB after opening")).toBeVisible();
  await page.getByRole("button", { name: "Apply the snapshot" }).click();
  await expect(page.getByRole("heading", { name: "Apply result" })).toBeVisible();
  await expect(page.getByText("Downloaded again for apply: 1.9 MB")).toBeVisible();

  localChanges = true;
  await page.reload();
  await expect(page.getByRole("heading", { name: "Previous success" })).toBeVisible();
  await page.getByText("Manage sync settings").click();
  refusePush = true;
  await page.getByRole("button", { name: "Push this workspace" }).click();
  await expect(page.getByRole("alert")).toContainText("update was cancelled");
  await expect(page.getByRole("alert")).toContainText("cancelled before retrying");
  await expect(page.getByRole("heading", { name: "Previous success" })).toBeVisible();
});

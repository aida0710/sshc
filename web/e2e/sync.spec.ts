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
  await expect(page.getByRole("alert")).toContainText("live snapshot was not updated");
  await expect(page.getByRole("alert")).toContainText("dated history copy");
  await expect(page.getByRole("heading", { name: "Previous success" })).toBeVisible();
});

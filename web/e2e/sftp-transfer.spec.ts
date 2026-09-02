import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

test("keeps a chunked SFTP upload visible while another section is open", async ({ page, installation }) => {
  test.setTimeout(process.env.SSHC_VISUAL_DIR === undefined ? 30_000 : 60_000);
  let releaseFirstChunk: (() => void) | undefined;
  const firstChunkGate = new Promise<void>((resolve) => { releaseFirstChunk = resolve; });
  let offset = 0;
  let chunks = 0;

  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ path: "/large", entries: [
      {
        name: "app", path: "/large/app", type: "directory", size: 0,
        modifiedAt: "2026-08-26T07:48:00Z", mode: "0755", revision: "app-v1",
      },
      {
        name: "archives", path: "/large/archives", type: "directory", size: 0,
        modifiedAt: "2026-08-21T11:20:00Z", mode: "0750", revision: "archives-v1",
      },
      {
        name: "project", path: "/large/project", type: "directory", size: 0,
        modifiedAt: "2026-08-31T10:49:00Z", mode: "0755", revision: "project-v1",
      },
      {
        name: "shared-datasets", path: "/large/shared-datasets", type: "directory", size: 0,
        modifiedAt: "2026-08-29T03:28:00Z", mode: "0775", revision: "shared-v1",
      },
      {
        name: "notes.txt", path: "/large/notes.txt", type: "file", size: 2048,
        modifiedAt: "2026-08-27T03:00:00Z", mode: "0644", revision: "notes-v1",
      },
    ] }),
  }));
  await page.route("**/api/v1/sftp/nas/entries**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ path: "/data", entries: [
      {
        name: "backups", path: "/data/backups", type: "directory", size: 0,
        modifiedAt: "2026-08-30T08:24:00Z", mode: "0750", revision: "backups-v1",
      },
      {
        name: "exports", path: "/data/exports", type: "directory", size: 0,
        modifiedAt: "2026-08-31T02:10:00Z", mode: "0755", revision: "exports-v1",
      },
      {
        name: "manifest.json", path: "/data/manifest.json", type: "file", size: 8192,
        modifiedAt: "2026-08-31T05:12:00Z", mode: "0640", revision: "manifest-v1",
      },
    ] }),
  }));
  await page.route("**/api/v1/sftp/bastion/text**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      entry: {
        name: "notes.txt", path: "/large/notes.txt", type: "file", size: 2048,
        modifiedAt: "2026-08-27T03:00:00Z", mode: "0644", revision: "notes-v1",
      },
      contents: "SFTP modal editor preview\n",
      revision: "notes-v1",
    }),
  }));
  await page.route("**/api/v1/sftp/bastion/preview**", (route) => route.fulfill({
    status: 415,
    contentType: "application/problem+json",
    body: JSON.stringify({ code: "sftp_preview_type", detail: "Text preview uses the text endpoint." }),
  }));
  await page.route("**/api/v1/sftp/bastion/uploads/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const id = url.pathname.split("/").at(-1) ?? "upload";
    if (url.pathname.endsWith("/complete")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ path: "/large.bin", bytes: offset, revision: "done" }) });
      return;
    }
    if (request.method() === "POST") {
      const body = request.postDataJSON() as { path: string; size: number };
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ id, path: body.path, offset, size: body.size, expectedRevision: "absent" }) });
      return;
    }
    if (request.method() === "PATCH") {
      if (chunks++ === 0) await firstChunkGate;
      offset += request.postDataBuffer()?.byteLength ?? 0;
      const total = Number(url.searchParams.get("total"));
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ id, path: url.searchParams.get("path"), offset, size: total, expectedRevision: "" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ changed: true }) });
  });

  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const chooseHost = async (alias: string, scope = page.getByRole("tabpanel")) => {
    await scope.locator("button[data-value]").click();
    const dialog = page.getByRole("dialog");
    await dialog.getByText(alias, { exact: true }).click();
  };
  await chooseHost("bastion");
  await page.locator('input[type="file"]:not([webkitdirectory])').first().setInputFiles({
    name: "large.bin",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc((2 << 20) + 17, 0x61),
  });
  const transferManager = page.getByRole("region", { name: "Transfer Manager" });
  await expect(transferManager.getByText(/1 active/)).toBeVisible();
  await transferManager.getByRole("button", { name: "Expand Transfer Manager" }).click();
  await expect(page.getByText("Transferring…")).toBeVisible();
  const fileListBounds = await page.getByRole("table").boundingBox();
  const transferManagerBounds = await page.getByRole("region", { name: "Transfer Manager" }).boundingBox();
  expect(transferManagerBounds?.y).toBeGreaterThan(fileListBounds?.y ?? Number.POSITIVE_INFINITY);
  await transferManager.getByRole("button", { name: "Collapse Transfer Manager" }).click();
  await expect(transferManager.getByText("large.bin", { exact: true })).toHaveCount(0);
  await transferManager.getByRole("button", { name: "Expand Transfer Manager" }).click();
  await expect(transferManager.getByText("large.bin", { exact: true })).toHaveCount(2);

  await openSection(page, "Connections");
  const sftpNavigation = page.getByRole("link", { name: "SFTP", exact: true });
  await expect(sftpNavigation.locator("[data-sftp-transfer-indicator]"))
    .toHaveAttribute("title", "1 active");
  await expect(page.getByRole("button", { name: "1 active" })).toHaveCount(0);
  await openSection(page, "SFTP");
  await expect(page.getByText("Transferring…")).toBeVisible();
  const reloadBastion = async () => {
    await chooseHost("nas");
    await chooseHost("bastion");
    await expect(page.getByRole("button", { name: "project", exact: true })).toBeVisible();
  };
  await reloadBastion();
  await expect(page.getByRole("button", { name: "notes.txt" })).toBeVisible();
  const project = page.getByRole("button", { name: "project", exact: true });
  await project.click();
  await expect(project).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Parent directory" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download" })).toBeVisible();
  await page.getByRole("checkbox", { name: "Select notes.txt" }).click();
  await expect(page.getByRole("button", { name: "Actions for 2 selected items" })).toBeEnabled();
  await page.getByRole("button", { name: "Actions for 2 selected items" }).click();
  await expect(page.getByRole("menuitem", { name: "Rename" })).toHaveCount(0);
  await page.getByRole("menuitem", { name: "Clear selection" }).click();

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await transferManager.getByRole("button", { name: "Collapse Transfer Manager" }).click();
    await page.screenshot({ path: `${visualDirectory}/sftp-polish-desktop-en.png`, fullPage: true });
    await transferManager.getByRole("button", { name: "Expand Transfer Manager" }).click();
    await page.screenshot({ path: `${visualDirectory}/transfer-manager-en.png`, fullPage: true });
    await transferManager.getByRole("button", { name: "Collapse Transfer Manager" }).click();
    await changeDisplayLanguage(page, "ja");
    await reloadBastion();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.evaluate(async () => { await document.fonts.ready; });
    await page.screenshot({ path: `${visualDirectory}/sftp-polish-desktop-ja.png`, fullPage: true });
    const japaneseTransferManager = page.getByRole("region", { name: "転送マネージャー" });
    await japaneseTransferManager.getByRole("button", { name: "転送マネージャーを展開" }).click();
    await page.screenshot({ path: `${visualDirectory}/transfer-manager-ja.png`, fullPage: true });
    await japaneseTransferManager.getByRole("button", { name: "転送マネージャーを折りたたむ" }).click();
    await page.getByRole("button", { name: "2ペイン" }).click();
    const secondPane = page.getByLabel("2つ目のリモートペイン");
    await chooseHost("nas", secondPane);
    await expect(secondPane.getByRole("button", { name: "backups" })).toBeVisible();
    await page.screenshot({ path: `${visualDirectory}/sftp-two-pane-desktop.png`, fullPage: true });
    await page.getByRole("button", { name: "1ペイン" }).click();
    await changeDisplayLanguage(page, "en");
    await reloadBastion();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-transfer-manager-desktop.png`, fullPage: true });
    await page.locator("button[data-value]").first().click();
    await expect(page.getByRole("dialog", { name: "Choose a remote host" })).toBeVisible();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-sftp-host-picker-desktop.png`, fullPage: true });
    await page.keyboard.press("Escape");
    const primaryFileArea = page.getByLabel("Upload files or folders to the current remote directory").first();
    const listWidth = (await primaryFileArea.boundingBox())?.width;
    await page.getByRole("button", { name: "notes.txt" }).dblclick();
    const detailsDialog = page.getByRole("dialog", { name: "Details for notes.txt" });
    await expect(detailsDialog).toBeVisible();
    await expect.poll(async () => (await primaryFileArea.boundingBox())?.width).toBe(listWidth);
    await expect(detailsDialog.getByText("SFTP modal editor preview")).toBeVisible();
    await expect(detailsDialog.getByRole("button", { name: "Edit file" })).toBeVisible();
    await expect(detailsDialog.getByRole("button", { name: "Download" })).toBeVisible();
    await expect(detailsDialog.getByRole("button", { name: "Rename" })).toBeVisible();
    const detailsBounds = await detailsDialog.boundingBox();
    expect(detailsBounds?.y).toBeGreaterThanOrEqual(0);
    expect(detailsBounds?.height).toBeLessThanOrEqual(page.viewportSize()?.height ?? 0);
    await page.waitForTimeout(300);
    await page.screenshot({ path: `${visualDirectory}/sftp-details-desktop.png`, fullPage: true });
    await detailsDialog.getByRole("button", { name: "Close" }).click();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.setViewportSize({ width: 360, height: 800 });
    await page.waitForTimeout(400);
    await expect(page.getByRole("list", { name: "Remote entries" })).toBeVisible();
    await page.getByRole("button", { name: "Actions for project" }).click();
    await expect(page.getByRole("menuitem", { name: "Download" })).toBeInViewport();
    await page.keyboard.press("Escape");
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-transfer-manager-mobile.png`, fullPage: true });
    await page.locator("button[data-value]").first().click();
    await expect(page.getByRole("dialog", { name: "Choose a remote host" })).toBeVisible();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-sftp-host-picker-mobile.png`, fullPage: true });
    await page.keyboard.press("Escape");
  }

  releaseFirstChunk?.();
  await expect.poll(() => offset).toBeGreaterThan(0);
  expect(chunks).toBeGreaterThanOrEqual(1);
});

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
  await page.getByRole("combobox", { name: "Host" }).selectOption("bastion");
  await page.locator('input[type="file"]:not([webkitdirectory])').setInputFiles({
    name: "large.bin",
    mimeType: "application/octet-stream",
    buffer: Buffer.alloc((2 << 20) + 17, 0x61),
  });
  await expect(page.getByText("Transferring…")).toBeVisible();
  const fileListBounds = await page.getByRole("table").boundingBox();
  const transferManagerBounds = await page.getByRole("region", { name: "Transfer Manager" }).boundingBox();
  expect(transferManagerBounds?.y).toBeGreaterThan(fileListBounds?.y ?? Number.POSITIVE_INFINITY);
  const transferManager = page.getByRole("region", { name: "Transfer Manager" });
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
    const host = page.getByRole("combobox");
    await host.selectOption("nas");
    await host.selectOption("bastion");
    await expect(page.getByRole("button", { name: "project", exact: true })).toBeVisible();
  };
  await reloadBastion();
  await expect(page.getByRole("button", { name: "notes.txt" })).toBeVisible();
  const project = page.getByRole("button", { name: "project", exact: true });
  await project.click();
  await expect(project).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Parent directory" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download" })).toHaveCount(0);
  await page.getByRole("checkbox", { name: "Select notes.txt" }).click();
  await expect(page.getByRole("button", { name: "Actions for 2 selected items" })).toBeEnabled();
  await page.getByRole("button", { name: "Actions for 2 selected items" }).click();
  await expect(page.getByRole("menuitem", { name: "Rename" })).toHaveCount(0);
  await page.getByRole("menuitem", { name: "Clear selection" }).click();

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/transfer-manager-en.png`, fullPage: true });
    await changeDisplayLanguage(page, "ja");
    await reloadBastion();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.evaluate(async () => { await document.fonts.ready; });
    await page.screenshot({ path: `${visualDirectory}/transfer-manager-ja.png`, fullPage: true });
    await changeDisplayLanguage(page, "en");
    await reloadBastion();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-transfer-manager-desktop.png`, fullPage: true });
    const listWidth = (await page.getByLabel("Upload files or folders to the current remote directory").boundingBox())?.width;
    await page.getByRole("button", { name: "notes.txt" }).dblclick();
    const editorDialog = page.getByRole("dialog", { name: "/large/notes.txt" });
    await expect(editorDialog).toBeVisible();
    await expect.poll(async () => (await page.getByLabel("Upload files or folders to the current remote directory").boundingBox())?.width).toBe(listWidth);
    await expect(page.getByText("Loading editor…")).toBeHidden();
    await expect(editorDialog.getByRole("button", { name: "Close" })).toBeVisible();
    const monacoBounds = await editorDialog.locator(".monaco-editor").boundingBox();
    expect(monacoBounds?.width).toBeGreaterThan(500);
    expect(monacoBounds?.height).toBeGreaterThan(300);
    const editorBounds = await editorDialog.boundingBox();
    expect(editorBounds?.y).toBeGreaterThanOrEqual(0);
    expect(editorBounds?.height).toBeLessThanOrEqual(page.viewportSize()?.height ?? 0);
    await page.waitForTimeout(300);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-sftp-editor-modal.png`, fullPage: true });
    await editorDialog.getByRole("button", { name: "Close" }).click();
    await page.getByRole("button", { name: "project", exact: true }).click();
    await page.setViewportSize({ width: 360, height: 800 });
    await page.waitForTimeout(400);
    await expect(page.getByRole("list", { name: "Remote entries" })).toBeVisible();
    await page.getByRole("button", { name: "Actions for project" }).click();
    await expect(page.getByRole("menuitem", { name: "Download" })).toBeInViewport();
    await page.keyboard.press("Escape");
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.16.1-transfer-manager-mobile.png`, fullPage: true });
  }

  releaseFirstChunk?.();
  await expect.poll(() => offset).toBeGreaterThan(0);
  expect(chunks).toBeGreaterThanOrEqual(1);
});

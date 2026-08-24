import { expect, openApplication, openSection, test } from "./support/environment";

test("keeps a chunked SFTP upload running while another section is open", async ({ page, installation }) => {
  let releaseFirstChunk: (() => void) | undefined;
  const firstChunkGate = new Promise<void>((resolve) => { releaseFirstChunk = resolve; });
  let offset = 0;
  let chunks = 0;

  await page.route("**/api/v1/sftp/bastion/entries?**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ path: "/", entries: [] }),
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

  await openSection(page, "Connections");
  releaseFirstChunk?.();
  await expect(page.getByText("Upload completed: large.bin")).toBeVisible();
  await openSection(page, "SFTP");
  await expect(page.getByText("Completed", { exact: true })).toBeVisible();
  expect(chunks).toBe(3);

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.0-transfer-manager-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 360, height: 800 });
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-v0.9.0-transfer-manager-mobile.png`, fullPage: true });
  }
});

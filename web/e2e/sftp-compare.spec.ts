import { join } from "node:path";
import { utimes } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";
import { openLocalSFTPDirectory, openSecondSFTPPane } from "./support/sftp";

test("compares engine-local directory metadata without offering a write operation", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  await installation.write("compare-left/only-left.txt", "left");
  await installation.write("compare-left/changed.txt", "short");
  await installation.write("compare-right/only-right.txt", "right");
  await installation.write("compare-right/changed.txt", "a different length");
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && new URL(request.url()).pathname.startsWith("/api/v1/sftp/")) {
      mutations.push(request.url());
    }
  });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openSecondSFTPPane(page, "New tab");
  for (const [index, directory] of ["compare-left", "compare-right"].entries()) {
    const pane = page.getByRole("tabpanel").nth(index);
    await openLocalSFTPDirectory({ page, pane, directory: join(installation.home, ".ssh", directory) });
    await expect(pane.getByRole("button", { name: "changed.txt", exact: true })).toBeVisible();
  }

  await page.getByRole("button", { name: "Compare directories", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Compare directories", exact: true });
  await expect(dialog).toContainText("Files are left unchanged.");
  await expect(dialog.getByRole("row", { name: /only-left.txt/ })).toContainText("Left only");
  await expect(dialog.getByRole("row", { name: /only-right.txt/ })).toContainText("Right only");
  await expect(dialog.getByRole("row", { name: /changed.txt/ })).toContainText("Changed");
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: /^Copy / })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(mutations).toEqual([]);
  expect(await installation.read("compare-left/changed.txt")).toBe("short");
  expect(await installation.read("compare-right/changed.txt")).toBe("a different length");
});

test("SHA256 comparison finds different contents with identical size and modification time", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  await installation.write("hash-left/equal-size.txt", "left value");
  await installation.write("hash-right/equal-size.txt", "right text");
  const modified = new Date("2026-01-01T00:00:00Z");
  for (const directory of ["hash-left", "hash-right"]) {
    await utimes(join(installation.home, ".ssh", directory, "equal-size.txt"), modified, modified);
  }
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && new URL(request.url()).pathname.startsWith("/api/v1/sftp/")) {
      mutations.push(request.url());
    }
  });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openSecondSFTPPane(page, "New tab");
  for (const [index, directory] of ["hash-left", "hash-right"].entries()) {
    const pane = page.getByRole("tabpanel").nth(index);
    await openLocalSFTPDirectory({ page, pane, directory: join(installation.home, ".ssh", directory) });
    await expect(pane.getByRole("button", { name: "equal-size.txt", exact: true })).toBeVisible();
  }

  await page.getByRole("button", { name: "Compare directories", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Compare directories", exact: true });
  await expect(dialog).toContainText("The two directories have matching metadata.");
  await dialog.getByRole("combobox", { name: "Compare mode", exact: true }).selectOption("content");
  await expect(dialog.getByRole("row", { name: /equal-size.txt/ })).toContainText("Changed");
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: /^Copy / })).toHaveCount(0);
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "content-comparison-en.png"), fullPage: true });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(mutations).toEqual([]);
  expect(await installation.read("hash-left/equal-size.txt")).toBe("left value");
  expect(await installation.read("hash-right/equal-size.txt")).toBe("right text");
});

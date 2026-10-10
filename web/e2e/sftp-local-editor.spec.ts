import { join } from "node:path";
import { mkdir } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";
import { openLocalSFTPDirectory, openSecondSFTPPane } from "./support/sftp";
import { watchForPolicyViolations } from "./support/policyViolations";

test("edits an engine-local file with Ctrl+S and confirms an external overwrite", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await installation.write("editor-local/notes.txt", "first line\n");
  const violations = watchForPolicyViolations(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await openLocalSFTPDirectory({ page, pane, directory: join(installation.home, ".ssh/editor-local") });
  await pane.getByRole("button", { name: "notes.txt", exact: true }).dblclick();
  await page.getByRole("dialog", { name: "Details for notes.txt", exact: true }).getByRole("button", { name: "Edit file", exact: true }).click();
  const editor = page.getByRole("dialog", { name: /editor-local\/notes.txt$/ });
  const input = editor.locator(".monaco-editor textarea");
  await input.press("Control+End");
  await input.pressSequentially("second line");
  await input.press("Control+s");
  await expect(editor.getByRole("status")).toHaveText("Saved");
  expect(await installation.read("editor-local/notes.txt")).toBe("first line\nsecond line");
  await input.pressSequentially(" mine");
  await installation.write("editor-local/notes.txt", "external change\n");
  await input.press("Control+s");
  await expect(editor.getByRole("alert")).toContainText("The local file changed or was replaced");
  expect(await installation.read("editor-local/notes.txt")).toBe("external change\n");
  await editor.getByRole("button", { name: "Overwrite", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "Overwrite the local file?", exact: true });
  await confirmation.getByRole("button", { name: "Overwrite", exact: true }).click();
  await expect(editor.getByRole("status")).toHaveText("Saved");
  expect(await installation.read("editor-local/notes.txt")).toBe("first line\nsecond line mine");
  await editor.getByRole("button", { name: "Close", exact: true }).click();
  expect(violations).toEqual([]);
});

test("shows line differences between two engine-local files without writing either", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await installation.write("text-left/notes.txt", "first line\nleft value\n");
  await installation.write("text-right/notes.txt", "first line\nright value\n");
  const violations = watchForPolicyViolations(page);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await openSecondSFTPPane(page, "New tab");
  for (const [index, directory] of ["text-left", "text-right"].entries()) {
    await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel").nth(index), directory: join(installation.home, ".ssh", directory) });
  }
  await page.getByRole("button", { name: "Compare directories", exact: true }).click();
  const comparison = page.getByRole("dialog", { name: "Compare directories", exact: true });
  await comparison.getByRole("button", { name: "View text differences for notes.txt", exact: true }).click();
  const text = page.getByRole("dialog", { name: "Text differences: notes.txt", exact: true });
  await expect(text.locator(".monaco-diff-editor")).toBeVisible();
  await expect(text.locator(".line-insert").first()).toBeVisible();
  await expect(text.locator(".line-delete").first()).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await mkdir(process.env.SSHC_VISUAL_DIR, { recursive: true });
    await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "sftp-text-differences-en.png") });
  }
  await text.getByRole("button", { name: "Close", exact: true }).click();
  await expect(comparison).toBeVisible();
  expect(await installation.read("text-left/notes.txt")).toBe("first line\nleft value\n");
  expect(await installation.read("text-right/notes.txt")).toBe("first line\nright value\n");
  expect(violations).toEqual([]);
  expect(errors).toEqual([]);
});

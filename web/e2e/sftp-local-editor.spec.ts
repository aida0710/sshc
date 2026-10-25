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
  const details = page.getByRole("dialog", { name: "Details for notes.txt", exact: true });
  // The details preview shows text in the read-only editor beside the properties.
  await expect(details.locator(".monaco-editor .view-lines")).toContainText("first line");
  await details.getByRole("button", { name: "Edit file", exact: true }).click();
  const editor = page.getByRole("dialog", { name: /editor-local\/notes.txt$/ });
  // Native EditContext exposes a textbox div and a hidden readonly textarea.
  // Use the accessible editor control in either Monaco input mode.
  const input = editor.getByRole("textbox", { name: "Editor content", exact: true });
  await input.focus();
  await page.keyboard.press("Control+End");
  await page.keyboard.insertText("second line");
  await expect(editor.getByRole("status")).toHaveText("Unsaved");
  await expect(editor.getByRole("button", { name: "Save", exact: true })).toBeEnabled();
  await page.keyboard.press("Control+s");
  await expect(editor.getByRole("status")).toHaveText("Saved");
  expect(await installation.read("editor-local/notes.txt")).toBe("first line\nsecond line");
  await input.focus();
  await page.keyboard.insertText(" mine");
  await expect(editor.getByRole("status")).toHaveText("Unsaved");
  await expect(editor.getByRole("button", { name: "Save", exact: true })).toBeEnabled();
  await installation.write("editor-local/notes.txt", "external change\n");
  await page.keyboard.press("Control+s");
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

test("finds and replaces text in an engine-local JSON file, then reopens it without page errors", async ({ page, installation }) => {
  const original = '{\n  "name": "alpha",\n  "port": \n}\n';
  await installation.write("editor-local/settings.json", original);
  const violations = watchForPolicyViolations(page);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  const pane = page.getByRole("tabpanel");
  await openLocalSFTPDirectory({ page, pane, directory: join(installation.home, ".ssh/editor-local") });

  const editor = page.getByRole("dialog", { name: /editor-local\/settings.json$/ });
  async function openSettings(): Promise<void> {
    await pane.getByRole("button", { name: "settings.json", exact: true }).dblclick();
    await page.getByRole("dialog", { name: "Details for settings.json", exact: true }).getByRole("button", { name: "Edit file", exact: true }).click();
    // The missing value is diagnosed by the JSON worker, after its lazy imports.
    await expect(editor.locator(".squiggly-error")).not.toHaveCount(0);
  }

  await openSettings();
  await editor.getByRole("textbox", { name: "Editor content", exact: true }).focus();
  await page.keyboard.press("Control+f");
  const findWidget = editor.getByRole("dialog", { name: "Find / Replace" });
  await expect(findWidget).toBeVisible();
  await page.keyboard.insertText("alpha");
  await expect(findWidget.getByText("1 of 1", { exact: true })).toBeVisible();
  await expect(editor.locator(".currentFindMatch")).toHaveCount(1);
  await page.keyboard.press("Control+h");
  await page.keyboard.insertText("omega");
  await findWidget.getByRole("button", { name: /^Replace All/ }).click();
  await page.keyboard.press("Escape");
  await expect(findWidget).toBeHidden();
  await editor.getByRole("button", { name: "Save", exact: true }).click();
  await expect(editor.getByRole("status")).toHaveText("Saved");
  expect(await installation.read("editor-local/settings.json")).toBe(original.replace("alpha", "omega"));
  await editor.getByRole("button", { name: "Close", exact: true }).click();

  await openSettings();
  await expect(editor.locator(".view-lines")).toContainText('"name": "omega"');
  await editor.getByRole("button", { name: "Close", exact: true }).click();
  expect(violations).toEqual([]);
  expect(errors).toEqual([]);
});

const comparisonFiles = [
  { name: "notes.txt", left: "first line\nleft value\n", right: "first line\nright value\n", isJSON: false },
  { name: "settings.json", left: '{\n  "name": "left"\n}\n', right: '{\n  "name": "right"\n}\n', isJSON: true },
];

for (const file of comparisonFiles) {
  test(`shows line differences for ${file.name} before opening any text editor, without errors or writes`, async ({ page, installation }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await installation.write(`text-left/${file.name}`, file.left);
    await installation.write(`text-right/${file.name}`, file.right);
    const violations = watchForPolicyViolations(page);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.stack ?? error.message));
    await openApplication(page, installation);
    await openSection(page, "SFTP");
    await openSecondSFTPPane(page, "New tab", "Local");
    for (const [index, directory] of ["text-left", "text-right"].entries()) {
      await openLocalSFTPDirectory({ page, pane: page.getByRole("tabpanel").nth(index), directory: join(installation.home, ".ssh", directory) });
    }
    await page.getByRole("button", { name: "Compare directories", exact: true }).click();
    const comparison = page.getByRole("dialog", { name: "Compare directories", exact: true });
    // Worker creation follows evaluation of the JSON client and its editor features.
    const jsonWorker = file.isJSON ? page.waitForEvent("worker", {
      predicate: (worker) => /\/json\.worker-[^/]+\.js$/.test(new URL(worker.url()).pathname),
    }) : undefined;
    await comparison.getByRole("button", { name: `View text differences for ${file.name}`, exact: true }).click();
    const text = page.getByRole("dialog", { name: `Text differences: ${file.name}`, exact: true });
    await expect(text.locator(".monaco-diff-editor")).toBeVisible();
    await expect(text.locator(".line-insert").first()).toBeVisible();
    await expect(text.locator(".line-delete").first()).toBeVisible();
    await jsonWorker;
    if (!file.isJSON && process.env.SSHC_VISUAL_DIR !== undefined) {
      await mkdir(process.env.SSHC_VISUAL_DIR, { recursive: true });
      await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "sftp-text-differences-en.png") });
    }
    await page.setViewportSize({ width: 390, height: 640 });
    await expect(text.locator(".line-insert").first()).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await text.getByRole("button", { name: "Close", exact: true }).click();
    await expect(comparison).toBeVisible();
    await comparison.getByRole("button", { name: "Cancel", exact: true }).click();
    await page.getByRole("button", { name: "Compare directories", exact: true }).click();
    const tabs = page.getByRole("dialog", { name: "Choose tabs to compare", exact: true });
    await expect(tabs).toBeVisible();
    await tabs.getByRole("button", { name: "Compare", exact: true }).click();
    await expect(comparison).toBeVisible();
    await comparison.getByRole("button", { name: `View text differences for ${file.name}`, exact: true }).click();
    await expect(text.locator(".monaco-diff-editor")).toBeVisible();
    // CodeLens and other contributions start after the first render or on idle.
    await page.evaluate(() => new Promise<void>((resolve) => { requestIdleCallback(() => resolve()); }));
    await text.getByRole("button", { name: "Close", exact: true }).click();
    expect(await installation.read(`text-left/${file.name}`)).toBe(file.left);
    expect(await installation.read(`text-right/${file.name}`)).toBe(file.right);
    expect(violations).toEqual([]);
    expect(errors).toEqual([]);
  });
}

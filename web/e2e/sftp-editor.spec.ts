import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";
import { watchForPolicyViolations } from "./support/policyViolations";
import { connectSFTPHost } from "./support/sftp";

// A remote file as the sshc engine would answer for it. A save names the
// revision it expects, and the engine refuses it as a conflict when the file is
// no longer that revision. A save waits for saveHeldUntil, when it is set.
type RemoteFile = { contents: string; revision: string; expectedRevisions: string[]; saveHeldUntil?: Promise<void> };

function remoteFile(contents: string): RemoteFile {
  return { contents, revision: "rev-1", expectedRevisions: [] };
}

function fileName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

// Serves `files`, by their path, as the only entries of /srv on bastion.
async function serveRemoteFiles(page: Page, files: Record<string, RemoteFile>): Promise<void> {
  const entries = Object.fromEntries(Object.entries(files).map(([path, file]) => [path, {
    name: fileName(path), path, type: "file", size: file.contents.length,
    modifiedAt: "2026-10-01T03:00:00Z", mode: "0644", revision: `${fileName(path)}-meta`,
  }]));
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ path: "/srv", entries: Object.values(entries) }),
  }));
  await page.route("**/api/v1/sftp/bastion/preview**", (route) => route.fulfill({
    status: 415,
    contentType: "application/problem+json",
    body: JSON.stringify({ code: "sftp_preview_type", message: "Text preview uses the text endpoint." }),
  }));
  await page.route("**/api/v1/sftp/bastion/text**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).searchParams.get("path") ?? "";
    const remote = files[path];
    if (remote === undefined) {
      await route.fulfill({
        status: 404,
        contentType: "application/problem+json",
        body: JSON.stringify({ code: "sftp_not_found", message: "No such file." }),
      });
      return;
    }
    if (request.method() === "PUT") {
      await remote.saveHeldUntil;
      const { contents, expectedRevision } = request.postDataJSON() as { contents: string; expectedRevision: string };
      remote.expectedRevisions.push(expectedRevision);
      if (expectedRevision !== remote.revision) {
        await route.fulfill({
          status: 409,
          contentType: "application/problem+json",
          body: JSON.stringify({ code: "sftp_conflict", message: "The remote file changed." }),
        });
        return;
      }
      remote.contents = contents;
      remote.revision = `saved-${remote.expectedRevisions.length}`;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ entry: entries[path], contents: remote.contents, revision: remote.revision }),
    });
  });
}

// The names the editor's controls have in one display language.
type EditorLabels = {
  details: (name: string) => string;
  editFile: string;
  unsaved: string;
  save: string;
  overwrite: string;
  overwriteHeading: string;
  cancel: string;
  close: string;
};

const englishLabels: EditorLabels = {
  details: (name) => `Details for ${name}`,
  editFile: "Edit file",
  unsaved: "Unsaved",
  save: "Save",
  overwrite: "Overwrite",
  overwriteHeading: "Overwrite the remote file?",
  cancel: "Cancel",
  close: "Close",
};

const japaneseLabels: EditorLabels = {
  details: (name) => `${name}の詳細`,
  editFile: "ファイルを編集",
  unsaved: "未保存",
  save: "保存",
  overwrite: "上書き保存",
  overwriteHeading: "リモートのファイルを上書き保存しますか？",
  cancel: "キャンセル",
  close: "閉じる",
};

// Opens the remote file at `path` in the editor the way a user does: from the
// file's details.
async function openInEditor(page: Page, path: string, labels: EditorLabels): Promise<Locator> {
  const name = fileName(path);
  await page.getByRole("button", { name, exact: true }).dblclick();
  await page.getByRole("dialog", { name: labels.details(name) }).getByRole("button", { name: labels.editFile }).click();
  return page.getByRole("dialog", { name: path });
}

// Opens notes.txt in the editor and types at its end. A change then lands on
// the remote before the save, so the engine refuses the save as a conflict.
async function editUntilConflict(page: Page, remote: RemoteFile, labels: EditorLabels): Promise<Locator> {
  const editor = await openInEditor(page, "/srv/notes.txt", labels);
  await editor.getByRole("textbox", { name: "Editor content" }).focus();
  await page.keyboard.press("ControlOrMeta+End");
  // One input event, so that "Unsaved" appears only once all of it is in. Keys
  // typed one by one can still be arriving when Save makes the editor read-only.
  await page.keyboard.insertText("mine");
  await expect(editor.getByText(labels.unsaved, { exact: true })).toBeVisible();
  Object.assign(remote, { contents: "changed elsewhere\n", revision: `${remote.revision}-elsewhere` });
  await editor.getByRole("button", { name: labels.save, exact: true }).click();
  await expect(editor.getByRole("alert").getByRole("button", { name: labels.overwrite })).toBeVisible();
  return editor;
}

// Presses Overwrite on the conflict and waits until the confirmation has read
// the remote file, so its own Overwrite button can be pressed.
async function openOverwriteConfirmation(page: Page, editor: Locator, labels: EditorLabels): Promise<Locator> {
  await editor.getByRole("alert").getByRole("button", { name: labels.overwrite }).click();
  const confirmation = page.getByRole("dialog", { name: labels.overwriteHeading });
  await expect(confirmation.getByRole("button", { name: labels.overwrite })).toBeEnabled();
  return confirmation;
}

test("overwrites a remote text file that changed after it was opened, only for the revision the user confirmed", async ({ page, installation }) => {
  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  test.setTimeout(visualDirectory === undefined ? 30_000 : 120_000);
  const remote = remoteFile("hello\n");
  await serveRemoteFiles(page, { "/srv/notes.txt": remote });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");

  const editor = await editUntilConflict(page, remote, englishLabels);
  const problem = editor.getByRole("alert");
  await expect(problem).toContainText("Could not save. The remote file changed after it was opened in the editor.");
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sftp-editor-conflict-desktop-en.png` });
    const desktop = page.viewportSize();
    await page.setViewportSize({ width: 360, height: 800 });
    await page.screenshot({ path: `${visualDirectory}/sftp-editor-conflict-narrow-en.png` });
    if (desktop !== null) await page.setViewportSize(desktop);
  }

  let confirmation = await openOverwriteConfirmation(page, editor, englishLabels);
  await expect(confirmation).toContainText("the changes made on the remote are lost");
  if (visualDirectory !== undefined) await page.screenshot({ path: `${visualDirectory}/sftp-editor-overwrite-confirm-desktop-en.png` });
  // Another change lands while the user reads the confirmation.
  Object.assign(remote, { contents: "changed again\n", revision: "rev-again" });
  await confirmation.getByRole("button", { name: englishLabels.overwrite }).click();
  await expect(problem).toContainText("Could not overwrite. The remote file changed again while the overwrite was being confirmed.");
  expect(remote.contents).toBe("changed again\n");

  confirmation = await openOverwriteConfirmation(page, editor, englishLabels);
  await confirmation.getByRole("button", { name: englishLabels.overwrite }).click();
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toHaveCount(0);
  await expect(problem).toHaveCount(0);
  expect(remote.contents).toBe("hello\nmine");
  expect(remote.expectedRevisions).toEqual(["rev-1", "rev-1-elsewhere", "rev-again"]);

  if (visualDirectory === undefined) return;
  await editor.getByRole("button", { name: englishLabels.close, exact: true }).click();
  await changeDisplayLanguage(page, "ja");
  await connectSFTPHost(page, "bastion");
  const japaneseEditor = await editUntilConflict(page, remote, japaneseLabels);
  await page.screenshot({ path: `${visualDirectory}/sftp-editor-conflict-desktop-ja.png` });
  await page.setViewportSize({ width: 360, height: 800 });
  await page.screenshot({ path: `${visualDirectory}/sftp-editor-conflict-narrow-ja.png` });
  const japaneseConfirmation = await openOverwriteConfirmation(page, japaneseEditor, japaneseLabels);
  await page.screenshot({ path: `${visualDirectory}/sftp-editor-overwrite-confirm-narrow-ja.png` });
  await japaneseConfirmation.getByRole("button", { name: japaneseLabels.cancel }).click();
});

// A line that looks like HTML. The editor builds the lines it draws as HTML,
// so this line shows whether the file's text reaches the page as text.
const htmlLookingLine = "<b>not bold</b> & <img src=x>";

// More lines than the editor shows at once, so it draws only some of them.
const manyLines = Array.from({ length: 300 }, (_, index) => `line ${index + 1}`);

test("draws the opened file's lines as text while the file is typed in and scrolled, and the page reports no policy violation", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  await serveRemoteFiles(page, { "/srv/notes.txt": remoteFile([htmlLookingLine, ...manyLines].join("\n")) });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  const drawnLines = editor.locator(".view-lines");
  await expect(drawnLines).toContainText(htmlLookingLine);
  await expect(drawnLines.locator("b, img")).toHaveCount(0);

  await editor.getByRole("textbox", { name: "Editor content" }).focus();
  await page.keyboard.insertText("typed ");
  await expect(drawnLines).toContainText(`typed ${htmlLookingLine}`);

  // The editor draws only the lines in view, so scrolling draws lines it had
  // not drawn before. Monaco scrolls a fixed step for each wheel event,
  // whatever its delta, so the wheel turns until the line comes into view.
  await expect(drawnLines).not.toContainText("line 60");
  const bounds = await editor.locator(".monaco-editor").boundingBox();
  if (bounds === null) throw new Error("the editor is not visible");
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  await expect(async () => {
    await page.mouse.wheel(0, 600);
    await expect(drawnLines).toContainText("line 60", { timeout: 200 });
  }).toPass({ intervals: [100] });
  await expect(drawnLines).not.toContainText(htmlLookingLine);
  await page.keyboard.press("ControlOrMeta+End");
  await expect(drawnLines).toContainText("line 300");
  await page.keyboard.press("ControlOrMeta+Home");
  await expect(drawnLines).toContainText(`typed ${htmlLookingLine}`);

  expect(violations).toEqual([]);
});

test("runs the JSON support and shows the read-only message while a save is held, and the page reports no policy violation", async ({ page, installation }) => {
  const violations = watchForPolicyViolations(page);
  const settings = remoteFile('{\n  "name": "sshc",\n  "port": \n}\n');
  let releaseSave = () => {};
  settings.saveHeldUntil = new Promise<void>((release) => { releaseSave = release; });
  await serveRemoteFiles(page, { "/srv/settings.json": settings });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");

  // A JSON file loads Monaco's JSON support. The missing value is found by the
  // JSON worker, so its squiggle shows that the worker started.
  let editor = await openInEditor(page, "/srv/settings.json", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText('"name": "sshc"');
  await expect(editor.locator(".squiggly-error")).not.toHaveCount(0);

  // The editor features the JSON support loads, among them the message for
  // typing into a read-only editor, apply to the editors created after it. The
  // file is therefore opened again.
  await editor.getByRole("button", { name: englishLabels.close, exact: true }).click();
  editor = await openInEditor(page, "/srv/settings.json", englishLabels);
  const content = editor.getByRole("textbox", { name: "Editor content" });
  await content.focus();
  await page.keyboard.insertText(" ");
  const save = editor.getByRole("button", { name: englishLabels.save, exact: true });
  await save.click();
  // The editor is read-only while the save is held. Monaco draws its message
  // with its Markdown renderer, which sanitizes it with DOMPurify.
  await expect(save).toBeDisabled();
  await content.focus();
  await page.keyboard.press("x");
  await expect(editor.getByText("Cannot edit in read-only editor")).toBeVisible();
  releaseSave();
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toHaveCount(0);

  expect(violations).toEqual([]);
});

import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";
import { connectSFTPHost } from "./support/sftp";

const notesEntry = {
  name: "notes.txt", path: "/srv/notes.txt", type: "file", size: 6,
  modifiedAt: "2026-10-01T03:00:00Z", mode: "0644", revision: "notes-meta",
};

// The remote notes.txt as the sshc engine would answer for it. A save names
// the revision it expects, and the engine refuses it as a conflict when the
// file is no longer that revision.
type RemoteNotes = { contents: string; revision: string; expectedRevisions: string[] };

async function serveRemoteNotes(page: Page, remote: RemoteNotes): Promise<void> {
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ path: "/srv", entries: [notesEntry] }),
  }));
  await page.route("**/api/v1/sftp/bastion/preview**", (route) => route.fulfill({
    status: 415,
    contentType: "application/problem+json",
    body: JSON.stringify({ code: "sftp_preview_type", message: "Text preview uses the text endpoint." }),
  }));
  await page.route("**/api/v1/sftp/bastion/text**", async (route) => {
    const request = route.request();
    if (request.method() === "PUT") {
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
      body: JSON.stringify({ entry: notesEntry, contents: remote.contents, revision: remote.revision }),
    });
  });
}

// The names the editor's controls have in one display language.
type EditorLabels = {
  details: string;
  editFile: string;
  unsaved: string;
  save: string;
  overwrite: string;
  overwriteHeading: string;
  cancel: string;
  close: string;
};

const englishLabels: EditorLabels = {
  details: "Details for notes.txt",
  editFile: "Edit file",
  unsaved: "Unsaved",
  save: "Save",
  overwrite: "Overwrite",
  overwriteHeading: "Overwrite the remote file?",
  cancel: "Cancel",
  close: "Close",
};

const japaneseLabels: EditorLabels = {
  details: "notes.txtの詳細",
  editFile: "ファイルを編集",
  unsaved: "未保存",
  save: "保存",
  overwrite: "上書き保存",
  overwriteHeading: "リモートのファイルを上書き保存しますか？",
  cancel: "キャンセル",
  close: "閉じる",
};

// Opens notes.txt in the editor and types at its end. A change then lands on
// the remote before the save, so the engine refuses the save as a conflict.
async function editUntilConflict(page: Page, remote: RemoteNotes, labels: EditorLabels): Promise<Locator> {
  await page.getByRole("button", { name: "notes.txt" }).dblclick();
  await page.getByRole("dialog", { name: labels.details }).getByRole("button", { name: labels.editFile }).click();
  const editor = page.getByRole("dialog", { name: "/srv/notes.txt" });
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
  const remote: RemoteNotes = { contents: "hello\n", revision: "rev-1", expectedRevisions: [] };
  await serveRemoteNotes(page, remote);
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

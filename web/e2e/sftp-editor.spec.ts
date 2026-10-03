import type { Locator, Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test, type Installation } from "./support/environment";
import { pressShortcutAndReportPrevented } from "./support/keyboard";
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

// Opens the application on SFTP, connected to bastion, whose /srv holds only
// `files`.
async function openSFTPOnBastion(page: Page, installation: Installation, files: Record<string, RemoteFile>): Promise<void> {
  await serveRemoteFiles(page, files);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");
}

// What the page reports that the editor must not cause: the errors the page
// throws, such as Monaco's for an editor feature it cannot start, and the
// Content Security Policy violations, Trusted Types included.
type PageProblems = { errors: string[]; policyViolations: string[] };

const noPageProblems: PageProblems = { errors: [], policyViolations: [] };

// Collects the problems the page reports from now on.
function watchForPageProblems(page: Page): PageProblems {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  return { errors, policyViolations: watchForPolicyViolations(page) };
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

// The text area Monaco types into. Monaco names it in English in either
// display language.
function editorContent(editor: Locator): Locator {
  return editor.getByRole("textbox", { name: "Editor content" });
}

// Monaco takes its keys from the platform the user agent names, and the suite
// runs as Desktop Chrome on Windows (playwright.config.ts). These are the keys
// Monaco answers to there, whichever system runs the suite. Playwright's
// ControlOrMeta is not used for them: it presses Meta when the suite runs on
// macOS, where Monaco, still reading Windows from the user agent, expects
// Control.
const endOfFileKey = "Control+End";
const startOfFileKey = "Control+Home";
const findKey = "Control+F";
const replaceKey = "Control+H";
const goToLineKey = "Control+G";
const toggleCommentKey = "Control+/";
const suggestKey = "Control+Space";

// Opens notes.txt in the editor and types at its end. A change then lands on
// the remote before the save, so the engine refuses the save as a conflict.
async function editUntilConflict(page: Page, remote: RemoteFile, labels: EditorLabels): Promise<Locator> {
  const editor = await openInEditor(page, "/srv/notes.txt", labels);
  await editorContent(editor).focus();
  await page.keyboard.press(endOfFileKey);
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
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remote });

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
  const problems = watchForPageProblems(page);
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remoteFile([htmlLookingLine, ...manyLines].join("\n")) });

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  const drawnLines = editor.locator(".view-lines");
  await expect(drawnLines).toContainText(htmlLookingLine);
  await expect(drawnLines.locator("b, img")).toHaveCount(0);

  await editorContent(editor).focus();
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
  await page.keyboard.press(endOfFileKey);
  await expect(drawnLines).toContainText("line 300");
  await page.keyboard.press(startOfFileKey);
  await expect(drawnLines).toContainText(`typed ${htmlLookingLine}`);

  expect(problems).toEqual(noPageProblems);
});

test("keeps every key typed in quick succession where it was typed", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const remote = remoteFile("first line\n");
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remote });

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText("first line");
  await editorContent(editor).focus();
  await page.keyboard.press(endOfFileKey);
  // One key after another as fast as the browser takes them, so that a key
  // often lands before React has finished with the one before it.
  await page.keyboard.type("the quick brown fox jumps over the lazy dog");
  await page.keyboard.press("Enter");
  await page.keyboard.type("second line");

  await editor.getByRole("button", { name: englishLabels.save, exact: true }).click();
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toHaveCount(0);
  expect(remote.contents).toBe("first line\nthe quick brown fox jumps over the lazy dog\nsecond line");
  expect(problems).toEqual(noPageProblems);
});

// The part of MonacoEnvironment a script on the page could call to ask for a
// Trusted Types policy.
type PolicySource = { createTrustedTypesPolicy?(name: string, options: object): unknown };

test("a script on the page gets none of the editor's Trusted Types policies through MonacoEnvironment", async ({ page, installation }) => {
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remoteFile("hello\n") });
  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText("hello");

  // Monaco has created editorViewLayer, and the worker policy is sshc's own.
  // Asking the environment for either name again must not hand out a policy
  // that lets any string through.
  const answers = await page.evaluate(() => {
    const environment = (globalThis as { MonacoEnvironment?: PolicySource }).MonacoEnvironment;
    const ask = (name: string) => {
      try {
        return environment?.createTrustedTypesPolicy?.(name, { createHTML: (value: string) => value, createScriptURL: (value: string) => value }) === undefined ? "none" : "policy";
      } catch {
        return "refused";
      }
    };
    return { installed: environment !== undefined, editorViewLayer: ask("editorViewLayer"), defaultWorkerFactory: ask("defaultWorkerFactory") };
  });
  expect(answers).toEqual({ installed: true, editorViewLayer: "refused", defaultWorkerFactory: "none" });
});

test("runs the JSON support and shows the read-only message while a save is held, and the page reports no policy violation", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const settings = remoteFile('{\n  "name": "sshc",\n  "port": \n}\n');
  let releaseSave = () => {};
  settings.saveHeldUntil = new Promise<void>((release) => { releaseSave = release; });
  await openSFTPOnBastion(page, installation, { "/srv/settings.json": settings });

  // A JSON file loads Monaco's JSON support. The missing value is found by the
  // JSON worker, so its squiggle shows that the worker started.
  const editor = await openInEditor(page, "/srv/settings.json", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText('"name": "sshc"');
  await expect(editor.locator(".squiggly-error")).not.toHaveCount(0);

  const content = editorContent(editor);
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

  expect(problems).toEqual(noPageProblems);
});

// The word the Find and Replace tests look for. Each of their files has it
// three times.
const searchedWord = "alpha";

// In the open editor, finds searchedWord with Ctrl+F, replaces every match
// with `replacement` through Ctrl+H, and closes Find with Escape, the way a
// user does. Escape must close only Find: the editor stays open, and does not
// ask whether to discard the replaced text.
async function findAndReplaceAll(page: Page, editor: Locator, replacement: string): Promise<void> {
  const drawnLines = editor.locator(".view-lines");
  const findWidget = editor.getByRole("dialog", { name: "Find / Replace" });
  await editorContent(editor).focus();
  expect(await pressShortcutAndReportPrevented(page, findKey)).toBe(true);
  await expect(findWidget).toBeVisible();
  await page.keyboard.insertText(searchedWord);
  await expect(findWidget.getByText("1 of 3", { exact: true })).toBeVisible();
  await expect(editor.locator(".view-overlays").locator(".findMatch, .currentFindMatch")).toHaveCount(3);

  expect(await pressShortcutAndReportPrevented(page, replaceKey)).toBe(true);
  await page.keyboard.insertText(replacement);
  await findWidget.getByRole("button", { name: /^Replace All/ }).click();
  await expect(drawnLines).not.toContainText(searchedWord);
  await expect(drawnLines.getByText(replacement)).toHaveCount(3);

  await page.keyboard.press("Escape");
  await expect(findWidget).toBeHidden();
  await expect(editor).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(1);
}

test("finds and replaces text in the editor with Ctrl+F and Ctrl+H, and Escape closes only Find", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remoteFile(`${searchedWord} one\ntwo ${searchedWord}\n${searchedWord} three\n`) });

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText(`${searchedWord} one`);
  await findAndReplaceAll(page, editor, "omega");
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toBeVisible();

  expect(problems).toEqual(noPageProblems);
});

test("finds and replaces text the same way after the JSON support has loaded, and the page throws no error", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const settings = [`{`, `  "first": "${searchedWord}",`, `  "second": "${searchedWord}",`, `  "third": "${searchedWord}",`, `  "port": `, `}`, ``];
  await openSFTPOnBastion(page, installation, { "/srv/settings.json": remoteFile(settings.join("\n")) });

  // The JSON support imports every editor feature too. A feature that only the
  // JSON support registered would start only in the editors created after the
  // JSON support loaded, and those editors would throw "depends on UNKNOWN
  // service". The file is therefore opened again, and Find and Replace are used
  // in this second editor. The missing value is found by the JSON worker, so
  // its squiggle shows that the JSON support has loaded.
  let editor = await openInEditor(page, "/srv/settings.json", englishLabels);
  await expect(editor.locator(".squiggly-error")).not.toHaveCount(0);
  await editor.getByRole("button", { name: englishLabels.close, exact: true }).click();
  editor = await openInEditor(page, "/srv/settings.json", englishLabels);
  await findAndReplaceAll(page, editor, "omega");

  expect(problems).toEqual(noPageProblems);
});

test("goes to a line with Ctrl+G and comments it out with Ctrl+/, and Escape closes only the command palette and the context menu", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const script = ["#!/bin/sh", "set -eu", "echo first", "echo second", "echo third", ""];
  await openSFTPOnBastion(page, installation, { "/srv/deploy.sh": remoteFile(script.join("\n")) });

  const editor = await openInEditor(page, "/srv/deploy.sh", englishLabels);
  const drawnLines = editor.locator(".view-lines");
  await expect(drawnLines).toContainText("echo second");
  await editorContent(editor).focus();

  // The cursor starts on line 1, so only Go to Line puts the comment on line 4.
  expect(await pressShortcutAndReportPrevented(page, goToLineKey)).toBe(true);
  await expect(editor.getByRole("textbox", { name: /^Go to line\./ })).toBeFocused();
  await page.keyboard.insertText("4");
  await page.keyboard.press("Enter");
  await page.keyboard.press(toggleCommentKey);
  await expect(drawnLines).toContainText("# echo second");

  await page.keyboard.press("F1");
  const commandPalette = editor.getByRole("textbox", { name: "Type to narrow down results." });
  await expect(commandPalette).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(commandPalette).toBeHidden();

  // Monaco draws the context menu in a shadow root inside the editor.
  await drawnLines.click({ button: "right" });
  const contextMenu = editor.getByRole("menu");
  await expect(contextMenu.getByRole("menuitem", { name: /^Command Palette/ })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(contextMenu).toBeHidden();

  await expect(page.getByRole("dialog")).toHaveCount(1);
  expect(problems).toEqual(noPageProblems);
});

// The pause between keys when a test types at a person's pace. A suggestion
// that Monaco shows while typing comes 10 ms after a key and a request to the
// editor worker, well within this pause.
const typingPause = 150;

test("starts a new line when Enter follows the start of a word in the file, and completes a word only on Ctrl+Space", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const remote = remoteFile("alphabet soup\nサーバーの設定を変更した。\n");
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remote });

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText("alphabet soup");
  await editorContent(editor).focus();
  await page.keyboard.press(endOfFileKey);
  // Monaco takes Japanese text up to a space or an ASCII punctuation mark for
  // one word, so the word in the file that starts with "サーバー" is the whole
  // sentence. Each character is entered as an input method commits it.
  await page.keyboard.type("サーバー", { delay: typingPause });
  await page.keyboard.press("Enter");
  await page.keyboard.type("alp", { delay: typingPause });
  await page.keyboard.press("Enter");

  await page.keyboard.type("sou");
  await page.keyboard.press(suggestKey);
  await expect(editor.getByRole("listbox", { name: "Suggest" }).getByRole("listitem", { name: /^soup,/ })).toBeVisible();
  await page.keyboard.press("Enter");

  await editor.getByRole("button", { name: englishLabels.save, exact: true }).click();
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toHaveCount(0);
  expect(remote.contents).toBe("alphabet soup\nサーバーの設定を変更した。\nサーバー\nalp\nsoup");
  expect(problems).toEqual(noPageProblems);
});

// Waits until the page has been idle. Monaco starts most of its editor
// features once the page is idle after it creates an editor, at the latest
// 50 ms after.
async function waitUntilPageIsIdle(page: Page): Promise<void> {
  await page.evaluate(() => new Promise<void>((resolve) => { requestIdleCallback(() => resolve()); }));
}

test("opens a file with unusual line terminators as it is, without asking to remove them", async ({ page, installation }) => {
  const problems = watchForPageProblems(page);
  const dialogs: string[] = [];
  // Accepted, so that an offer to remove the terminators would also show as an
  // unsaved change.
  page.on("dialog", (dialog) => {
    dialogs.push(dialog.message());
    void dialog.accept();
  });
  // U+2028 is Line Separator, one of the unusual line terminators.
  await openSFTPOnBastion(page, installation, { "/srv/notes.txt": remoteFile("first\u2028second\nthird\n") });

  const editor = await openInEditor(page, "/srv/notes.txt", englishLabels);
  await expect(editor.locator(".view-lines")).toContainText("third");
  await waitUntilPageIsIdle(page);

  expect(dialogs).toEqual([]);
  await expect(editor.getByText(englishLabels.unsaved, { exact: true })).toHaveCount(0);
  expect(problems).toEqual(noPageProblems);
});

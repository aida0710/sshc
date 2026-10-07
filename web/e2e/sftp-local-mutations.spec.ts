import { mkdir, readFile, symlink, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

// Two language changes and real filesystem requests share one isolated engine.
const localMutationTestTimeout = 60_000;

async function openDeleteConfirmation(page: Page, selectionLabel: string, deleteLabel: string) {
  await page.getByRole("button", { name: selectionLabel, exact: true }).click();
  await page.getByRole("menu", { name: selectionLabel, exact: true }).getByRole("menuitem", { name: deleteLabel, exact: true }).click();
}

test("creates, renames and deletes only the selected local fixtures, with cancellable confirmations in both languages", async ({ page, installation }) => {
  test.setTimeout(localMutationTestTimeout);
  await page.setViewportSize({ width: 1400, height: 900 });
  const directory = join(installation.home, "files");
  const keep = join(installation.home, "keep.txt");
  await mkdir(directory, { mode: 0o700 });
  await writeFile(keep, "outside the selected tree", { mode: 0o600 });
  await writeFile(join(directory, "notes.txt"), "notes", { mode: 0o600 });
  await symlink(keep, join(directory, "shortcut"));
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Host", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: /Local.*sshc engine/ }).click();
  await page.getByRole("button", { name: "files", exact: true }).dblclick();
  await expect(page.getByRole("button", { name: "notes.txt", exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Create or upload" }).click();
  await expect(page.getByRole("menuitem", { name: "New empty file" })).toHaveCount(0);
  await page.getByRole("menuitem", { name: "New folder" }).click();
  const create = page.getByRole("dialog", { name: "New folder" });
  await create.getByRole("textbox", { name: "Folder name" }).fill("created");
  await create.getByRole("button", { name: "New folder" }).click();
  await expect(page.getByRole("button", { name: "created", exact: true })).toBeVisible();
  await writeFile(join(directory, "created", "inside.txt"), "inside", { mode: 0o600 });

  await page.getByRole("button", { name: "notes.txt", exact: true }).click();
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  const rename = page.getByRole("dialog", { name: "Rename" });
  await rename.getByRole("textbox", { name: "New name" }).fill("renamed.txt");
  await rename.getByRole("button", { name: "Rename", exact: true }).click();
  await expect(page.getByRole("button", { name: "renamed.txt", exact: true })).toBeVisible();
  await expect.poll(() => readFile(join(directory, "renamed.txt"), "utf8")).toBe("notes");
  await page.getByRole("button", { name: "renamed.txt", exact: true }).click();

  await openDeleteConfirmation(page, "Actions for renamed.txt", "Delete");
  const english = page.getByRole("dialog", { name: "Delete this local entry?" });
  await expect(english).toContainText("machine running the sshc engine");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "local-delete-en.png"), fullPage: true });
  await english.getByRole("button", { name: "Cancel" }).click();
  expect(await readFile(join(directory, "renamed.txt"), "utf8")).toBe("notes");

  await changeDisplayLanguage(page, "ja");
  await expect(page.getByRole("button", { name: "renamed.txt", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "ディレクトリを更新" }).click();
  await page.getByRole("button", { name: "renamed.txt", exact: true }).click();
  await page.getByRole("button", { name: "created", exact: true }).click({ modifiers: ["ControlOrMeta"] });
  await page.getByRole("button", { name: "shortcut", exact: true }).click({ modifiers: ["ControlOrMeta"] });
  await openDeleteConfirmation(page, "選択した3件の操作", "削除");
  const japanese = page.getByRole("dialog", { name: "選択したローカル項目3件を削除しますか？" });
  await expect(japanese).toContainText("sshcエンジンが動いているマシン");
  await expect(japanese).toContainText("フォルダ内の項目もすべて削除されます。");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: join(process.env.SSHC_VISUAL_DIR, "local-delete-ja.png"), fullPage: true });
  await japanese.getByRole("button", { name: "削除", exact: true }).click();
  await expect(japanese).toBeHidden();
  for (const name of ["renamed.txt", "created", "shortcut"]) await expect(page.getByRole("button", { name, exact: true })).toHaveCount(0);
  expect(await readFile(keep, "utf8")).toBe("outside the selected tree");
});

test("refuses a changed local selection and keeps the confirmation and fixture", async ({ page, installation }) => {
  const filename = join(installation.home, "notes.txt");
  await writeFile(filename, "before", { mode: 0o600 });
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await page.getByRole("button", { name: "Host", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: /Local.*sshc engine/ }).click();
  await page.getByRole("button", { name: "notes.txt", exact: true }).click();
  await openDeleteConfirmation(page, "Actions for notes.txt", "Delete");
  const confirmation = page.getByRole("dialog", { name: "Delete this local entry?" });
  await writeFile(filename, "changed after confirmation opened", { mode: 0o600 });
  await confirmation.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(confirmation.getByRole("alert")).toContainText("A local entry changed or is in use by a transfer.");
  expect(await readFile(filename, "utf8")).toBe("changed after confirmation opened");
  await confirmation.getByRole("button", { name: "Cancel" }).click();
});

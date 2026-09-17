import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

test("selects the pinned Local destination beside an SSH host", async ({ page, installation }) => {
  test.setTimeout(60_000);
  // Wide enough for two panes to each keep the full table rather than the compact list.
  await page.setViewportSize({ width: 1800, height: 900 });
  await page.route("**/api/v1/sftp/bastion/entries**", (route) => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({ path: "/srv/projects", entries: [
      { name: "app", path: "/srv/projects/app", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-09-17T08:00:00Z", revision: "app" },
      { name: "README.md", path: "/srv/projects/README.md", type: "file", size: 1208, mode: "0644", modifiedAt: "2026-09-17T08:00:00Z", revision: "readme" },
    ] }),
  }));
  await page.route("**/api/v1/sftp/local/entries**", (route) => route.fulfill({
    status: 200, contentType: "application/json",
    body: JSON.stringify({ path: "/home/engine/projects", home: "/home/engine", entries: [
      { name: "notes", path: "/home/engine/projects/notes", type: "directory", size: 0, mode: "drwxr-xr-x", modifiedAt: "2026-09-17T07:30:00Z", revision: "notes" },
      { name: "draft.txt", path: "/home/engine/projects/draft.txt", type: "file", size: 344, mode: "-rw-r--r--", modifiedAt: "2026-09-17T07:45:00Z", revision: "draft" },
    ] }),
  }));
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await changeDisplayLanguage(page, "ja");
  const first = page.getByLabel("1つ目のリモートペイン");
  await first.getByRole("button", { name: "ホスト" }).click();
  await page.getByRole("dialog").getByText("bastion", { exact: true }).click();
  await first.getByRole("button", { name: "接続" }).click();
  await expect(first.getByRole("button", { name: "README.md" })).toBeVisible();
  await page.getByRole("button", { name: "2ペイン" }).click();
  const second = page.getByLabel("2つ目のリモートペイン");
  await second.getByRole("button", { name: "ホスト" }).click();
  const picker = page.getByRole("dialog");
  await expect(picker.getByRole("button", { name: /ローカル.*sshc/ })).toBeVisible();
  await picker.getByRole("button", { name: /ローカル.*sshc/ }).click();
  await expect(second.getByRole("button", { name: "draft.txt" })).toBeVisible();
  // Both panes render the same table: the local side has the same columns as the remote one.
  for (const pane of [first, second]) {
    const table = pane.getByRole("table");
    await expect(table.getByRole("columnheader", { name: /名前/ })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: /更新日時/ })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "権限" })).toBeVisible();
  }
  await expect(second.getByRole("row", { name: /draft.txt/ })).toContainText("-rw-r--r--");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/local-shared-toolbar-ja.png`, fullPage: true });
  // Selecting a local row shows the same selection bar as the remote side, with upload as its action.
  await second.getByRole("button", { name: "draft.txt" }).click();
  await expect(second.getByText("選択中：draft.txt")).toBeVisible();
  await expect(second.getByRole("button", { name: "選択項目をアップロード" })).toBeEnabled();
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/local-shared-selection-ja.png`, fullPage: true });
  await second.getByRole("button", { name: "選択を解除" }).click();
  await second.getByRole("button", { name: "ローカルパスを編集" }).click();
  await expect(second.getByRole("textbox", { name: "エンジン側のファイルパス" })).toHaveValue("/home/engine/projects");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/local-shared-toolbar-path-ja.png`, fullPage: true });
  await page.setViewportSize({ width: 390, height: 800 });
  await first.getByRole("button", { name: "ホスト" }).click();
  await page.getByRole("dialog").getByRole("button", { name: /ローカル.*sshc/ }).click();
  await expect(first.getByRole("button", { name: "draft.txt" })).toBeVisible();
  // On a phone the local side falls back to the same two-line list as a remote host.
  await expect(first.getByRole("list", { name: "ファイル一覧" }).getByRole("button", { name: "draft.txt" })).toContainText("-rw-r--r--");
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/local-shared-mobile-pane-ja.png`, fullPage: true });
  await first.getByRole("button", { name: "フォルダ操作" }).click();
  await expect(page.getByRole("dialog", { name: "フォルダ操作" }).getByRole("menuitem", { name: "ホームディレクトリ" })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR) await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/local-shared-mobile-ja.png`, fullPage: true });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
});

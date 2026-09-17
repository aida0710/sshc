import { changeDisplayLanguage, expect, openApplication, openSection, test } from "./support/environment";

test("selects the pinned Local destination beside an SSH host", async ({ page, installation }) => {
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1440, height: 900 });
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
      { name: "notes", path: "/home/engine/projects/notes", type: "directory", size: 0 },
      { name: "draft.txt", path: "/home/engine/projects/draft.txt", type: "file", size: 344 },
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
  await second.getByRole("button", { name: "ローカルパスを編集" }).click();
  await expect(second.getByRole("textbox", { name: "エンジン側のファイルパス" })).toHaveValue("/home/engine/projects");
});

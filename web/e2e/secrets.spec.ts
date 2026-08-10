import {
  expect,
  masterPassword,
  openApplication,
  openSection,
  sessionStatus,
  test,
} from "./support/environment";

// この機能全体が存在する理由となる構成を、使い捨ての HOME に対して
// ビルド済みバイナリで動かす。1 つの名前の下にある 1 つの secret、それを
// 指す 2 つのホスト、そしてどちらの名前も含まないディスク上のファイル。
test("gives one named secret to two hosts and writes neither name into the file", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  const passwords = page.getByRole("region", { name: "Account passwords" });
  await expect(passwords).toBeVisible();
  await passwords.getByLabel("New account password name").fill("office-vm");
  await passwords.getByLabel("New account password value").fill("hunter2");
  await passwords.getByRole("button", { name: "Store account password" }).click();

  await expect(passwords.getByRole("button", { name: "Delete office-vm" })).toBeVisible();
  // 一覧が示すのは名前とそれを使うものであり、値は決して示さない。
  await expect(page.locator("body")).not.toContainText("hunter2");

  // 2 つのホスト、1 つの名前。それぞれが自分自身の画面から
  // それを選び、そこがホストのパスワードを選ぶ唯一の場所だ。
  for (const alias of ["bastion", "nas"]) {
    await openSection(page, "Connections");
    await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: alias }).click();
    await expect(page.getByRole("heading", { name: alias, exact: true })).toBeVisible();

    const panel = page.getByRole("region", { name: "Authentication" });
    await panel.getByLabel("Stored password action").selectOption("saved_password");
    await panel.getByLabel("Saved password").selectOption("office-vm");
    const saved = page.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/v1/connections" && response.request().method() === "PATCH",
    );
    await page.getByRole("button", { name: "Save Basic settings" }).click();
    expect((await saved).status()).toBe(200);
    await expect(panel.getByText("Assigned: office-vm")).toBeVisible();
  }

  await openSection(page, "Secrets");
  await expect(page.getByRole("region", { name: "Account passwords" })).toContainText("bastion, nas");

  // そして封印されたファイルはそのどれも含まない。secret も、
  // 名前も、それを指すホストも含まない。
  const sealed = await installation.read("sshc/secrets");
  for (const absent of ["hunter2", "office-vm", "bastion", "nas"]) {
    expect(sealed).not.toContain(absent);
  }
});

// このピッカーに鍵のパスフレーズが入っていれば、次の接続でログインパス
// ワードとしてリモートホストへ送られてしまう。2 つの名前空間が 2 つに分かれ
// ているのは、どの画面もそれについて気を配らずに済むようにするためだ。
test("never offers a key passphrase where a host password is chosen", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  const phrases = page.getByRole("region", { name: "Key passphrases" });
  await phrases.getByLabel("New key passphrase name").fill("build-key");
  await phrases.getByLabel("New key passphrase value").fill("a passphrase");
  await phrases.getByRole("button", { name: "Store key passphrase" }).click();
  await expect(phrases.getByRole("button", { name: "Delete build-key" })).toBeVisible();

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();

  const panel = page.getByRole("region", { name: "Authentication" });
  await panel.getByLabel("Stored password action").selectOption("saved_password");
  // アカウントパスワードはそもそも存在しないため、ピッカーは
  // 誤った種類のものを提供する場として存在しない。
  await expect(panel.getByLabel("Saved password").locator("option")).toHaveCount(1);
  await expect(panel.getByRole("option", { name: "build-key" })).toHaveCount(0);
});

test("keeps application controls in Settings and changes the master password there", async ({
  page,
  installation,
}) => {
  const nextMasterPassword = "a replacement end to end password";
  await openApplication(page, installation);

  await openSection(page, "Secrets");
  await expect(page.getByRole("region", { name: "Start at login" })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Master password" })).toHaveCount(0);

  await openSection(page, "Settings");
  await expect(page).toHaveURL(/\/settings$/);
  const master = page.getByRole("region", { name: "Master password" });
  await expect(master).toBeVisible();
  await master.getByLabel("Current master password", { exact: true }).fill(masterPassword);
  await master.getByLabel("New master password", { exact: true }).fill(nextMasterPassword);
  await master.getByLabel("Confirm new master password", { exact: true }).fill(nextMasterPassword);

  const changed = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/passwords/change" &&
      response.request().method() === "POST",
  );
  await master.getByRole("button", { name: "Change the master password" }).click();
  expect((await changed).status()).toBe(200);
  await expect(master.getByRole("status")).toContainText("Changed on this machine");
  await expect(master.getByLabel("Current master password", { exact: true })).toHaveValue("");
  await expect(master.getByLabel("New master password", { exact: true })).toHaveValue("");
  await expect(master.getByLabel("Confirm new master password", { exact: true })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText(masterPassword);
  await expect(page.locator("body")).not.toContainText(nextMasterPassword);

  await openSection(page, "Secrets");
  await page.getByRole("button", { name: "Lock sshc" }).click();
  await page.getByLabel("Master password", { exact: true }).fill(nextMasterPassword);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(sessionStatus(page)).toContainText("Local session active");
});

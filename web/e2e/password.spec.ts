import { clickAndAwait, expect, openApplication, openSection, test } from "./support/environment";

// この end-to-end スイートは使い捨ての HOME に対してビルド済みバイナリ
// を動かすため、実ディスク上の本物の vault ファイルを検証する。terminal
// も ssh も決して起動しない。askpass ヘルパー自体の振る舞いは
// cmd/sshc のテストが、償還規則は internal/secret のテストがカバーする。
test("stores a password for a host and never shows it again", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();

  const panel = page.getByRole("region", { name: "Authentication" });
  await expect(panel).toBeVisible();
  await expect(panel.getByLabel("SSH private key")).toHaveValue("");

  await panel.getByLabel("Stored password action").selectOption("dedicated_password");
  await panel.getByRole("textbox", { name: "Connection password", exact: true }).fill("hunter2");
  const saved = page.waitForResponse(
    (response) => new URL(response.url()).pathname === "/api/v1/connections" && response.request().method() === "PATCH",
  );
  await page.getByRole("button", { name: "Save Basic settings" }).click();
  expect((await saved).status()).toBe(200);

  await expect(panel.getByText(/connection-only password is assigned/)).toBeVisible();
  await expect(page.locator("body")).not.toContainText("hunter2");

  // そしてディスク上のファイルは暗号文であり、パスワードも alias も含まない。
  const sealed = await installation.read("sshc/secrets");
  expect(sealed).not.toContain("hunter2");
  expect(sealed).not.toContain("bastion");
});

test("selecting a key removes the stored password assignment and returning to agent auth permits a new one", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();

  let panel = page.getByRole("region", { name: "Authentication" });
  await panel.getByLabel("Stored password action").selectOption("dedicated_password");
  await panel.getByRole("textbox", { name: "Connection password", exact: true }).fill("first e2e password");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);

  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_exclusive_auth");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();
  panel = page.getByRole("region", { name: "Authentication" });
  const keyChoice = panel.getByLabel("SSH private key");
  const keyID = await keyChoice.getByRole("option", { name: /id_exclusive_auth/ }).getAttribute("value");
  expect(keyID).not.toBeNull();

  await keyChoice.selectOption(keyID!);
  await expect(panel.getByLabel("Stored password action")).toHaveCount(0);
  await expect(panel.getByText(/will remove this connection's assignment/)).toBeVisible();
  const connectionURL = page.url();
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  await expect(page).toHaveURL(connectionURL);
  await expect(panel.getByText(/will remove this connection's assignment/)).toHaveCount(0);
  await expect(panel.getByLabel("Stored password action")).toHaveCount(0);
  expect(await installation.read("config")).toContain("IdentityFile ~/.ssh/id_exclusive_auth");

  await keyChoice.selectOption("");
  await expect(panel.getByLabel("Stored password action")).toBeVisible();
  await panel.getByLabel("Stored password action").selectOption("dedicated_password");
  await panel.getByRole("textbox", { name: "Connection password", exact: true }).fill("replacement e2e password");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  expect(await installation.read("config")).not.toContain("IdentityFile");
  await expect(page.locator("body")).not.toContainText("replacement e2e password");
});

// vault をロックするとアプリケーション全体がロックされる。かつては 1 つのパネルだ
// けがロックされ、ユーザーは次の要求が拒否されるシェルの中に取り残されていた。
test("locking the vault returns the application to its front door", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  await page.getByRole("button", { name: "Lock sshc" }).click();

  await expect(page.getByLabel("Master password", { exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toHaveCount(0);
});

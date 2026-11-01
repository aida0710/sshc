import {
  expect,
  masterPassword,
  openApplication,
  openSection,
  openSettingsPage,
  sessionStatus,
  test,
} from "./support/environment";
import type { Page } from "@playwright/test";

// addCredential stores one credential through the page's ［＋ Add］ form, which opens above the list.
async function addCredential(page: Page, form: string, name: string, valueLabel: string, value: string) {
  await page.getByRole("button", { name: "Add", exact: true }).click();
  const panel = page.getByRole("region", { name: form });
  await panel.getByLabel("Name", { exact: true }).fill(name);
  await panel.getByLabel(valueLabel, { exact: true }).fill(value);
  await panel.getByRole("button", { name: "Save", exact: true }).click();
  await expect(panel).toHaveCount(0);
}

// openRowMenuItem chooses an item from a row's "…" menu (the menu is portalled outside the list).
async function openRowMenuItem(page: Page, row: string, item: string) {
  await page.getByRole("button", { name: `Actions for ${row}` }).click();
  await page.getByRole("menuitem", { name: item }).click();
}

test("gives one named secret to two hosts and writes neither name into the file", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Account passwords");

  const passwords = page.getByRole("region", { name: "Account passwords" });
  await expect(passwords).toBeVisible();
  await addCredential(page, "Add an account password", "office-vm", "Password", "hunter2");

  await expect(passwords.getByRole("button", { name: "Actions for office-vm" })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("hunter2");

  for (const alias of ["bastion", "nas"]) {
    await openSection(page, "Connections");
    await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: alias, exact: true }).click();
    await expect(page.getByRole("heading", { name: alias, exact: true })).toBeVisible();

    const panel = page.getByRole("region", { name: "Authentication" });
    await panel.getByLabel("Password", { exact: true }).selectOption("saved_password");
    await panel.getByLabel("Saved password").selectOption("office-vm");
    const saved = page.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/v1/connections" && response.request().method() === "PATCH",
    );
    await page.getByRole("button", { name: "Save Basic settings" }).click();
    expect((await saved).status()).toBe(200);
    await expect(panel.getByLabel("Password", { exact: true }).locator("option:checked")).toHaveText("office-vm (saved password)");
  }

  await openSection(page, "Account passwords");
  const office = page
    .getByRole("region", { name: "Account passwords" })
    .getByRole("article", { name: "office-vm" });
  await expect(office.getByText("bastion, nas").filter({ visible: true })).toBeVisible();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/vault-account-passwords.png`, fullPage: true });
  }

  const sealed = await installation.read("sshc/secrets");
  for (const absent of ["hunter2", "office-vm", "bastion", "nas"]) {
    expect(sealed).not.toContain(absent);
  }
});

test("never offers a key passphrase where a host password is chosen", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Key passphrases");

  const phrases = page.getByRole("region", { name: "Key passphrases" });
  await addCredential(page, "Add a key passphrase", "build-key", "Key passphrase", "a passphrase");
  await expect(phrases.getByRole("button", { name: "Actions for build-key" })).toBeVisible();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
    await page.reload();
    await expect(page.getByRole("region", { name: "鍵のパスフレーズ" })).toBeVisible();
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/vault-key-passphrases.png`, fullPage: true });
    await page.evaluate(() => window.localStorage.setItem("sshc.language", "en"));
    await page.reload();
    await expect(page.getByRole("region", { name: "Key passphrases" })).toBeVisible();
  }

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).click();

  const panel = page.getByRole("region", { name: "Authentication" });
  await panel.getByLabel("Password", { exact: true }).selectOption("saved_password");
  await expect(panel.getByLabel("Saved password").locator("option")).toHaveCount(1);
  await expect(panel.getByRole("option", { name: "build-key" })).toHaveCount(0);
});

test("stores and assigns a TOTP seed without exposing it in the page or vault file", async ({
  page,
  installation,
}) => {
  const setupKey = "JBSWY3DPEHPK3PXP";
  await openApplication(page, installation);
  await openSection(page, "OTP");

  const tokens = page.getByRole("region", {
    name: "One-time passwords (TOTP)",
  });
  await addCredential(page, "Add a one-time password", "production-otp", "Base32 setup key or otpauth URI", setupKey);
  const token = tokens.getByRole("article", { name: "production-otp" });
  await expect(token).toBeVisible();
  await expect(token).toContainText(/\d{3} \d{3}/);
  await expect(tokens.getByText("Previous")).toHaveCount(0);
  await openRowMenuItem(page, "production-otp", "Show the codes before and after");
  await expect(tokens.getByText("Previous")).toBeVisible();
  await expect(tokens.getByText("Next")).toBeVisible();

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).click();
  const authentication = page.getByRole("region", { name: "Authentication" });
  await authentication.getByLabel("One-time password", { exact: true }).selectOption("saved_totp");
  await authentication.getByLabel("Saved TOTP").selectOption("production-otp");
  const saved = page.waitForResponse(
    (response) => new URL(response.url()).pathname === "/api/v1/connections" && response.request().method() === "PATCH",
  );
  await page.getByRole("button", { name: "Save Basic settings" }).click();
  expect((await saved).status()).toBe(200);
  await expect(authentication.getByLabel("One-time password", { exact: true }).locator("option:checked")).toHaveText("production-otp");

  await openSection(page, "OTP");
  await openRowMenuItem(page, "production-otp", "Assigned hosts (1)");
  await expect(page.getByRole("list", { name: "Assigned hosts" })).toContainText("bastion");
  await expect(page.locator("body")).not.toContainText(setupKey);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    for (const name of ["aws dubguild sso", "mdx"]) {
      await addCredential(page, "Add a one-time password", name, "Base32 setup key or otpauth URI", setupKey);
      await expect(tokens.getByRole("article", { name })).toBeVisible();
    }
    await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
    await page.reload();
    await expect(
      page.getByRole("region", { name: "ワンタイムパスワード（TOTP）" }),
    ).toBeVisible();
    await page.setViewportSize({ width: 1280, height: 1200 });
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/totp-vault-current.png`,
      fullPage: true,
    });
    await page.getByRole("button", { name: "production-otpの操作" }).click();
    await page.getByRole("menuitem", { name: "前後のコードを表示" }).click();
    await expect(page.getByText("ひとつ前")).toBeVisible();
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/totp-vault-expanded.png`,
      fullPage: true,
    });
  }

  const sealed = await installation.read("sshc/secrets");
  for (const absent of [setupKey, "production-otp", "bastion"]) {
    expect(sealed).not.toContain(absent);
  }
});

test("opens a named password masked and reveals it only on request", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Account passwords");

  const passwords = page.getByRole("region", { name: "Account passwords" });
  await addCredential(page, "Add an account password", "office-vm", "Password", "original-test-password");
  await expect(passwords.getByRole("article", { name: "office-vm" })).toBeVisible();
  await openRowMenuItem(page, "office-vm", "Edit");

  // The form opens under the row, not as a dialog over the page.
  const dialog = passwords.getByRole("region", { name: "Edit account password" });
  const password = dialog.getByLabel("Password", { exact: true });
  await expect(dialog.getByLabel("Name")).toHaveValue("office-vm");
  await expect(password).toHaveValue("original-test-password");
  await expect(password).toHaveAttribute("type", "password");

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.screenshot({ path: `${visualDirectory}/sshc-credential-edit-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    const navigation = page.getByRole("button", { name: "Navigation" });
    if (await navigation.getAttribute("aria-expanded") === "true") await navigation.click();
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${visualDirectory}/sshc-credential-edit-mobile.png`, fullPage: true });
  }

  await dialog.getByRole("button", { name: "Show Password" }).click();
  await expect(password).toHaveAttribute("type", "text");
  await dialog.getByLabel("Name").fill("office-shared");
  await password.fill("rotated-test-password");
  await dialog.getByRole("button", { name: "Save changes" }).click();

  await expect(page.getByRole("article", { name: "office-shared" })).toBeVisible();
  await expect(page.getByRole("article", { name: "office-vm" })).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText("rotated-test-password");
});

test("keeps application controls in Settings and changes the master password there", async ({
  page,
  installation,
}) => {
  const nextMasterPassword = "a replacement end to end password";
  await openApplication(page, installation);

  await openSection(page, "Account passwords");
  await expect(page.getByRole("region", { name: "Master password" })).toHaveCount(0);

  await openSettingsPage(page, "Vault");
  await expect(page).toHaveURL(/\/settings\/vault$/);
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
  await expect(master.getByRole("status")).toContainText("Local vault protection was updated, including snippets, sync settings, and backups.");
  await expect(master.getByLabel("Current master password", { exact: true })).toHaveValue("");
  await expect(master.getByLabel("New master password", { exact: true })).toHaveValue("");
  await expect(master.getByLabel("Confirm new master password", { exact: true })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText(masterPassword);
  await expect(page.locator("body")).not.toContainText(nextMasterPassword);

  await openSection(page, "Account passwords");
  await page.getByRole("button", { name: "Lock sshc" }).click();
  await page.getByLabel("Master password", { exact: true }).fill(nextMasterPassword);
  await page.getByRole("button", { name: "Open" }).click();
  await expect(sessionStatus(page)).toContainText("Local session active");
});

test("saves the Vault auto-lock policy and warns when automatic locking is disabled", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSettingsPage(page, "Vault");

  const autoLock = page.getByRole("region", { name: "Vault auto-lock" });
  const policy = autoLock.getByLabel("Vault auto-lock");
  await expect(policy).toHaveValue("idle");
  await expect(autoLock.getByLabel("Time")).toHaveValue("12");
  await expect(autoLock.getByLabel("Unit")).toHaveValue("hours");

  // A choice is saved as it is made; the page says so beside its name.
  const saved = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/settings/presets/default" &&
      response.request().method() === "PUT",
  );
  await policy.selectOption("restart");
  await expect(autoLock.getByText(
    "The Vault will not lock automatically. Unless you lock it manually, it remains unlocked until sshc is restarted. Use this setting only on a device you control.",
  )).toBeVisible();
  expect((await saved).status()).toBe(200);
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.reload();
  const restored = page.getByRole("region", { name: "Vault auto-lock" });
  await expect(restored.getByLabel("Vault auto-lock")).toHaveValue("restart");
  await expect(restored.getByText("The Vault will not lock automatically.", { exact: false })).toBeVisible();

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await restored.scrollIntoViewIfNeeded();
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.screenshot({ path: `${visualDirectory}/vault-auto-lock-desktop.png`, fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    const navigation = page.getByRole("button", { name: "Navigation" });
    if (await navigation.getAttribute("aria-expanded") === "true") await navigation.click();
    await restored.scrollIntoViewIfNeeded();
    await page.waitForTimeout(400);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await page.screenshot({ path: `${visualDirectory}/vault-auto-lock-mobile.png`, fullPage: true });
  }
});

test("lists saved one-time passwords with their current codes in both languages", async ({ page, installation }) => {
  // Published example and RFC 6238 test keys; the screen shows only the derived codes.
  const setupKeys = {
    "bastion-otp": "JBSWY3DPEHPK3PXP",
    "production-otp": "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
    "staging-otp": "MFRGGZDFMZTWQ2LKNNWG23TPOBYXE43U",
  };
  await openApplication(page, installation);
  await openSection(page, "OTP");

  const tokens = page.getByRole("region", { name: "One-time passwords (TOTP)" });
  for (const [name, setupKey] of Object.entries(setupKeys)) {
    await addCredential(page, "Add a one-time password", name, "Base32 setup key or otpauth URI", setupKey);
    await expect(tokens.getByRole("article", { name })).toContainText(/\d{3} \d{3}/);
  }

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion", exact: true }).click();
  const authentication = page.getByRole("region", { name: "Authentication" });
  await authentication.getByLabel("One-time password", { exact: true }).selectOption("saved_totp");
  await authentication.getByLabel("Saved TOTP").selectOption("bastion-otp");
  await page.getByRole("button", { name: "Save Basic settings" }).click();
  await expect(authentication.getByLabel("One-time password", { exact: true }).locator("option:checked")).toHaveText("bastion-otp");
  await openSection(page, "OTP");
  await expect(page.getByRole("article", { name: "bastion-otp" })).toBeVisible();
  for (const setupKey of Object.values(setupKeys)) {
    await expect(page.locator("body")).not.toContainText(setupKey);
  }

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/otp-desktop-en.png` });
    await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
    await page.reload();
    await expect(page.getByRole("region", { name: "ワンタイムパスワード（TOTP）" })).toBeVisible();
    await expect(page.getByRole("article", { name: "bastion-otp" })).toContainText(/\d{3} \d{3}/);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/otp-desktop-ja.png` });
  }
});

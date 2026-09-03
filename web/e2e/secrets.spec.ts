import {
  expect,
  masterPassword,
  openApplication,
  openSection,
  openSettingsPage,
  sessionStatus,
  test,
} from "./support/environment";

test("gives one named secret to two hosts and writes neither name into the file", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  const passwords = page.getByRole("region", { name: "Account passwords" });
  await expect(passwords).toBeVisible();
  await passwords.getByLabel("New account password name").fill("office-vm");
  await passwords.getByLabel("New account password value", { exact: true }).fill("hunter2");
  await passwords.getByRole("button", { name: "Store account password" }).click();

  await expect(passwords.getByRole("button", { name: "Delete office-vm" })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("hunter2");

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
  const office = page
    .getByRole("region", { name: "Account passwords" })
    .getByRole("article", { name: "office-vm" });
  const assignedHosts = office.getByRole("list", { name: "Assigned hosts" });
  await expect(assignedHosts.getByRole("listitem")).toHaveText(["bastion", "nas"]);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/credentials-desktop.png`, fullPage: true });
  }

  const sealed = await installation.read("sshc/secrets");
  for (const absent of ["hunter2", "office-vm", "bastion", "nas"]) {
    expect(sealed).not.toContain(absent);
  }
});

test("never offers a key passphrase where a host password is chosen", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  const phrases = page.getByRole("region", { name: "Key passphrases" });
  await phrases.getByLabel("New key passphrase name").fill("build-key");
  await phrases.getByLabel("New key passphrase value", { exact: true }).fill("a passphrase");
  await phrases.getByRole("button", { name: "Store key passphrase" }).click();
  await expect(phrases.getByRole("button", { name: "Delete build-key" })).toBeVisible();

  await openSection(page, "Connections");
  await page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "bastion" }).click();

  const panel = page.getByRole("region", { name: "Authentication" });
  await panel.getByLabel("Stored password action").selectOption("saved_password");
  await expect(panel.getByLabel("Saved password").locator("option")).toHaveCount(1);
  await expect(panel.getByRole("option", { name: "build-key" })).toHaveCount(0);
});

test("stores and assigns a TOTP seed without exposing it in the page or vault file", async ({
  page,
  installation,
}) => {
  const setupKey = "JBSWY3DPEHPK3PXP";
  await openApplication(page, installation);
  await openSection(page, "Secrets");

  const tokens = page.getByRole("region", {
    name: "One-time passwords (TOTP)",
  });
  await tokens.getByLabel("New one-time password name").fill("production-otp");
  await tokens.getByLabel("Base32 setup key or otpauth URI", { exact: true }).fill(setupKey);
  await tokens.getByRole("button", { name: "Store one-time password" }).click();
  await expect(tokens.getByRole("article", { name: "production-otp" })).toBeVisible();

  await tokens.getByLabel("Host alias").fill("bastion");
  await tokens.locator("select").selectOption("production-otp");
  await tokens.getByRole("button", { name: "Assign to host" }).click();
  const token = tokens.getByRole("article", { name: "production-otp" });
  await expect(token.getByRole("list", { name: "Assigned hosts" })).toContainText("bastion");
  await expect(page.locator("body")).not.toContainText(setupKey);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.evaluate(() => window.localStorage.setItem("sshc.language", "ja"));
    await page.reload();
    await expect(
      page.getByRole("region", { name: "ワンタイムパスワード（TOTP）" }),
    ).toBeVisible();
    await page.setViewportSize({ width: 1280, height: 1200 });
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: `${process.env.SSHC_VISUAL_DIR}/totp-vault-desktop.png`,
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
  await openSection(page, "Secrets");

  const passwords = page.getByRole("region", { name: "Account passwords" });
  await passwords.getByLabel("New account password name").fill("office-vm");
  await passwords.getByLabel("New account password value", { exact: true }).fill("original-test-password");
  await passwords.getByRole("button", { name: "Store account password" }).click();
  await passwords.getByRole("button", { name: "Edit office-vm" }).click();

  const dialog = page.getByRole("dialog", { name: "Edit account password" });
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

  await openSection(page, "Secrets");
  await expect(page.getByRole("region", { name: "Master password" })).toHaveCount(0);

  await openSettingsPage(page, "Master password");
  await expect(page).toHaveURL(/\/settings\/password$/);
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
  await expect(master.getByRole("status")).toContainText("local vault, snippets, sync settings, and local backups");
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

test("saves the Vault auto-lock policy and warns when automatic locking is disabled", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSettingsPage(page, "Engine");

  const engine = page.getByRole("region", { name: "Engine" });
  const policy = engine.getByLabel("Vault auto-lock");
  await expect(policy).toHaveValue("idle");
  await expect(engine.getByLabel("Time")).toHaveValue("12");
  await expect(engine.getByLabel("Unit")).toHaveValue("hours");

  await policy.selectOption("restart");
  await expect(engine.getByText(
    "The Vault will not lock automatically. Unless you lock it manually, it remains unlocked until sshc is restarted. Use this setting only on a device you control.",
  )).toBeVisible();

  const saved = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/metadata/engine" &&
      response.request().method() === "PUT",
  );
  await engine.getByRole("button", { name: "Save", exact: true }).click();
  expect((await saved).status()).toBe(200);

  await page.reload();
  const restored = page.getByRole("region", { name: "Engine" });
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

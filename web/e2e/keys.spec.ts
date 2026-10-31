import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";
import { createGroups } from "./support/groups";
import { openKeyCreation } from "./support/keys";

test("hands a generated private key to one connection exactly once", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await openKeyCreation(page);
  await page.getByLabel("File name").fill("id_handoff_connection");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await page.getByRole("button", { name: "Assign to a connection" }).click();
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion", exact: true })
    .click();
  const key = page.getByLabel("SSH private key");
  await expect(key.getByRole("option", { name: /id_handoff_connection/ })).toHaveJSProperty(
    "selected",
    true,
  );
  await expect(page.getByText(/staged for this connection/)).toBeVisible();
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  expect(await installation.read("config")).toContain("IdentityFile ~/.ssh/id_handoff_connection");

  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "nas", exact: true })
    .click();
  await expect(page.getByLabel("SSH private key")).toHaveValue("");
  await expect(page.getByText(/staged for this connection/)).toHaveCount(0);
});

test("preloads a generated public key only on the requested server workflow", async ({
  page,
  installation,
}) => {
  // ~/.ssh の最初の公開鍵（名前の順）は、渡された鍵とは別のものにしておく。
  await installation.write(
    "id_another.pub",
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDCl0pXo+XohbTKwe3KeLjooZpC7lbbGsVOtwNHCUD1L another@fixture\n",
  );
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await openKeyCreation(page);
  await page.getByLabel("File name").fill("id_handoff_server");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await page.getByRole("button", { name: "Install on a server" }).click();
  await expect(page).toHaveURL(/\/install-key$/);
  const picker = page.getByLabel("Public key from ~/.ssh");
  await expect(picker.getByRole("option", { name: /^id_handoff_server\.pub/ })).toHaveJSProperty("selected", true);
  await expect(page.getByLabel("Public key line")).toHaveValue(/ssh-ed25519 /);

  // 渡された鍵は一度だけ使い、次に開いたときは ~/.ssh の最初の公開鍵を選ぶ。
  await openSection(page, "Keys");
  await openSection(page, "Install Key on Server");
  await expect(picker.getByRole("option", { name: /^id_another\.pub/ })).toHaveJSProperty("selected", true);
  await expect(page.getByLabel("Public key line")).toHaveValue(/another@fixture$/);
});

test("lists generated keys and reveals one only after an explicit confirmation", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await expect(
    page.getByRole("table", { name: "Files classified by content and permissions" }),
  ).toBeVisible();

  await openKeyCreation(page);

  await page.getByLabel("File name").fill("id_e2e");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_e2e\b/ }).first();
  await expect(row).toBeVisible();
  // The public half goes with its private key: no row of its own, and listed in the key's details, which
  // choosing the key opens.
  await expect(page.getByRole("button", { name: "id_e2e.pub", exact: true })).toHaveCount(0);
  await row.getByRole("button", { name: "id_e2e", exact: true }).click();
  const details = page.getByRole("complementary", { name: "Key details" });
  await expect(details).toContainText(process.platform === "win32" ? "0666" : "0600");
  await expect(details).toContainText("id_e2e.pub");
  expect(await installation.read("id_e2e.pub")).toContain("ssh-ed25519 ");

  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");

  await row.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Show private key" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveAttribute("aria-modal", "true");
  await expect(dialog.locator("xpath=ancestor::tr[@data-key-panel-for]")).toHaveCount(0);
  await expect(dialog.locator("xpath=ancestor::table")).toHaveCount(0);
  await expect(dialog.locator('pre[aria-label="Private key"]')).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/private-key-confirmation-modal.png`, fullPage: true });
  }

  await dialog.getByRole("button", { name: "Show private key" }).click();
  await expect(dialog.locator('pre[aria-label="Private key"]')).toContainText(
    "BEGIN OPENSSH PRIVATE KEY",
  );

  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");

  expect(await page.evaluate(() => Object.keys(window.localStorage))).toEqual(["sshc.browser.registration.v1"]);
  expect(await page.evaluate(() => Object.keys(window.sessionStorage))).toEqual(["sshc.session.csrf"]);
  expect(await page.evaluate(() => document.cookie)).toBe("");
});

test("stores an encrypted key passphrase from the key row and shows only its name afterwards", async ({
  page,
  installation,
}) => {
  const passphrase = "e2e key phrase that must leave the DOM";
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await openKeyCreation(page);
  await page.getByLabel("File name").fill("id_saved_phrase");
  await page.getByRole("textbox", { name: "Passphrase" }).fill(passphrase);
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_saved_phrase\b/ }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Save passphrase" }).click();
  await page.getByLabel("Passphrase name").fill("saved-e2e-key");
  await page.getByLabel("Passphrase value", { exact: true }).fill(passphrase);
  const assigned = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/credentials/key_passphrase/assign" &&
      response.request().method() === "PUT",
  );
  await page.getByRole("button", { name: "Save and use for this key" }).click();
  expect((await assigned).status()).toBe(200);

  await expect(page.getByLabel("Passphrase value", { exact: true })).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText(passphrase);
  await openSection(page, "Key passphrases");
  const passphrases = page.getByRole("region", { name: "Key passphrases" });
  await expect(passphrases).toContainText("saved-e2e-key");
  // The row names the keys the passphrase belongs to.
  await expect(passphrases.getByRole("article", { name: "saved-e2e-key" })).toContainText("id_saved_phrase");
  await expect(page.locator("body")).not.toContainText(passphrase);

  await openSection(page, "Keys");
  const assignedRow = page.getByRole("row", { name: /id_saved_phrase\b/ }).first();
  await assignedRow.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Rename or move" }).click();
  await page.getByLabel("Name", { exact: true }).fill("id_saved_phrase_renamed");
  expect(await clickAndAwait(page, "Rename or move the key", "/api/v1/keys/")).toBe(200);

  await openSection(page, "Key passphrases");
  await expect(passphrases).toContainText("id_saved_phrase_renamed");
  await expect(passphrases).not.toContainText(/\bid_saved_phrase\b(?!_renamed)/);
});

test("offers agent registration and refuses it honestly when no agent is reachable", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");

  await openKeyCreation(page);

  await page.getByLabel("File name").fill("id_agent");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_agent\b/ }).first();
  await expect(row).toBeVisible();

  await row.getByRole("button", { name: /^Actions for / }).click();
  const register = page.getByRole("menuitem", { name: "Add to ssh-agent" });
  await expect(register).toBeVisible();
  await expect(register).toBeDisabled();
  await expect(page.getByText(/This process cannot connect to ssh-agent/)).toBeVisible();
});

test("renames a key and carries every directive that named it", async ({ page, installation }) => {
  await installation.write(
    "conf.d/20-rename.conf",
    "Host renamed\n\tIdentityFile ~/.ssh/id_rename\n\tCertificateFile %d/.ssh/id_rename.pub\n",
  );
  await openApplication(page, installation);
  await openSection(page, "Keys");

  await openKeyCreation(page);

  await page.getByLabel("File name").fill("id_rename");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_rename\b/ }).first();
  await row.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Rename or move" }).click();
  await page.getByLabel("Name", { exact: true }).fill("id_renamed");
  expect(await clickAndAwait(page, "Rename or move the key", "/api/v1/keys/")).toBe(200);

  expect(await installation.read("id_renamed.pub")).toContain("ssh-ed25519 ");
  expect(await installation.read("conf.d/20-rename.conf")).toBe(
    "Host renamed\n\tIdentityFile ~/.ssh/id_renamed\n\tCertificateFile %d/.ssh/id_renamed.pub\n",
  );
  await expect(page.getByText("IdentityFile ~/.ssh/id_rename → ~/.ssh/id_renamed")).toBeVisible();
});

test("refuses a rename whose destination is taken, and writes nothing", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");

  await openKeyCreation(page);

  await page.getByLabel("File name").fill("id_first");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await openKeyCreation(page);

  await page.getByLabel("File name").fill("id_second");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const before = await installation.read("id_first");
  const row = page.getByRole("row", { name: /id_first\b/ }).first();
  await row.getByRole("button", { name: /^Actions for / }).click();
  await page.getByRole("menuitem", { name: "Rename or move" }).click();
  await page.getByLabel("Name", { exact: true }).fill("id_second");
  expect(await clickAndAwait(page, "Rename or move the key", "/api/v1/keys/")).toBe(409);

  await expect(page.getByRole("alert")).toContainText("already exists");
  expect(await installation.read("id_first")).toBe(before);
  expect(await installation.read("id_second.pub")).toContain("ssh-ed25519 ");
});

test("moves several chosen keys into a folder at once", async ({ page, installation }) => {
  await installation.write("conf.d/30-bulk.conf", "Host bulk\n\tIdentityFile ~/.ssh/id_bulk_a\n");
  await openApplication(page, installation);

  await createGroups(page, ["archive"]);

  await openSection(page, "Keys");
  for (const name of ["id_bulk_a", "id_bulk_b"]) {
    await openKeyCreation(page);
    await page.getByLabel("File name").fill(name);
    await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
    expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);
  }

  await page.getByRole("checkbox", { name: "Choose id_bulk_a" }).check();
  await page.getByRole("checkbox", { name: "Choose id_bulk_b" }).check();
  await page.getByRole("combobox", { name: "Move into" }).selectOption("archive");
  expect(await clickAndAwait(page, "Move", "/api/v1/keys/")).toBe(200);

  await expect(page.getByText("Moved 2.")).toBeVisible();
  expect(await installation.read("keys/archive/id_bulk_a.pub")).toContain("ssh-ed25519 ");
  expect(await installation.read("keys/archive/id_bulk_b.pub")).toContain("ssh-ed25519 ");
  expect(await installation.read("conf.d/30-bulk.conf")).toBe(
    "Host bulk\n\tIdentityFile ~/.ssh/keys/archive/id_bulk_a\n",
  );
});

test("shows one folder at a time", async ({ page, installation }) => {
  await openApplication(page, installation);
  await createGroups(page, ["archive"]);

  await openSection(page, "Keys");
  for (const name of ["id_kept", "id_filed"]) {
    await openKeyCreation(page);
    await page.getByLabel("File name").fill(name);
    await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
    expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);
  }
  await page.getByRole("checkbox", { name: "Choose id_filed" }).check();
  await page.getByRole("combobox", { name: "Move into" }).selectOption("archive");
  expect(await clickAndAwait(page, "Move", "/api/v1/keys/")).toBe(200);

  await page.getByRole("button", { name: /^archive,/ }).click();

  await expect(page.getByRole("row", { name: /id_filed/ }).first()).toBeVisible();
  await expect(page.getByRole("row", { name: /id_kept/ })).toHaveCount(0);
});

import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

test("hands a generated private key to one connection exactly once", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_handoff_connection");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await page.getByRole("button", { name: "Assign to a connection" }).click();
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
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
    .getByRole("button", { name: "nas" })
    .click();
  await expect(page.getByLabel("SSH private key")).toHaveValue("");
  await expect(page.getByText(/staged for this connection/)).toHaveCount(0);
});

test("preloads a generated public key only on the requested server workflow", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_handoff_server");
  await page.getByLabel(/Create without a passphrase/).check();
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await page.getByRole("button", { name: "Install on a server" }).click();
  await expect(page).toHaveURL(/\/install-key$/);
  await expect(page.getByLabel("Public key file")).toHaveValue("id_handoff_server.pub");
  await expect(page.getByLabel("Public key line")).toHaveValue(/ssh-ed25519 /);

  await openSection(page, "Keys");
  await openSection(page, "Install Key on Server");
  await expect(page.getByLabel("Public key file")).toHaveValue("");
  await expect(page.getByLabel("Public key line")).toHaveValue("");
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

  await page.getByLabel("File name").fill("id_e2e");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_e2e\b/ }).first();
  await expect(row).toBeVisible();
  const publicFiles = row.getByRole("button", { name: "Public key files (1)" });
  await expect(publicFiles).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("button", { name: "id_e2e.pub", exact: true })).toHaveCount(0);
  await publicFiles.click();
  const publicRow = page.getByRole("row", { name: /id_e2e\.pub/ });
  await expect(publicRow).toBeVisible();
  await expect(publicRow).toHaveAttribute("data-key-related-to");
  await publicFiles.click();
  await expect(publicRow).toHaveCount(0);

  await row.getByRole("button", { name: "id_e2e", exact: true }).click();
  await page.getByRole("button", { name: "Show Key details" }).click();
  const details = page.getByRole("complementary", { name: "Key details" });
  await expect(details).toContainText(process.platform === "win32" ? "0666" : "0600");
  expect(await installation.read("id_e2e.pub")).toContain("ssh-ed25519 ");

  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");

  await row.getByRole("button", { name: "Show private key" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('pre[aria-label="Private key"]')).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");

  await dialog.getByRole("button", { name: "Show private key" }).click();
  await expect(dialog.locator('pre[aria-label="Private key"]')).toContainText(
    "BEGIN OPENSSH PRIVATE KEY",
  );

  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(page.locator("body")).not.toContainText("BEGIN OPENSSH PRIVATE KEY");

  expect(
    await page.evaluate(() => ({
      local: window.localStorage.length,
      session: window.sessionStorage.length,
    })),
  ).toEqual({ local: 0, session: 0 });
  expect(await page.evaluate(() => document.cookie)).toBe("");
});

test("stores an encrypted key passphrase from the key row and shows only its name afterwards", async ({
  page,
  installation,
}) => {
  const passphrase = "e2e key phrase that must leave the DOM";
  await openApplication(page, installation);
  await openSection(page, "Keys");
  await page.getByLabel("File name").fill("id_saved_phrase");
  await page.getByRole("textbox", { name: "Passphrase" }).fill(passphrase);
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_saved_phrase\b/ }).first();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Save passphrase" }).click();
  await page.getByLabel("Passphrase name").fill("saved-e2e-key");
  await page.getByLabel("Passphrase value").fill(passphrase);
  const assigned = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/v1/credentials/key_passphrase/assign" &&
      response.request().method() === "PUT",
  );
  await page.getByRole("button", { name: "Save and use for this key" }).click();
  expect((await assigned).status()).toBe(200);

  await expect(page.getByLabel("Passphrase value")).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText(passphrase);
  await openSection(page, "Secrets");
  const passphrases = page.getByRole("region", { name: "Key passphrases" });
  await expect(passphrases).toContainText("saved-e2e-key");
  await expect(passphrases).toContainText("id_saved_phrase");
  await expect(page.locator("body")).not.toContainText(passphrase);

  await openSection(page, "Keys");
  const assignedRow = page.getByRole("row", { name: /id_saved_phrase\b/ }).first();
  await assignedRow.getByRole("button", { name: "More actions" }).click();
  await assignedRow.getByRole("button", { name: "Rename or move" }).click();
  await page.getByLabel("Name", { exact: true }).fill("id_saved_phrase_renamed");
  expect(await clickAndAwait(page, "Rename or move the key", "/api/v1/keys/")).toBe(200);

  await openSection(page, "Secrets");
  await expect(passphrases).toContainText("id_saved_phrase_renamed");
  await expect(passphrases).not.toContainText(/\bid_saved_phrase\b(?!_renamed)/);
});

test("offers agent registration and refuses it honestly when no agent is reachable", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Keys");

  await page.getByLabel("File name").fill("id_agent");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_agent\b/ }).first();
  await expect(row).toBeVisible();

  const register = row.getByRole("button", { name: "Add to ssh-agent" });
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

  await page.getByLabel("File name").fill("id_rename");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const row = page.getByRole("row", { name: /id_rename\b/ }).first();
  await row.getByRole("button", { name: "More actions" }).click();
  await row.getByRole("button", { name: "Rename or move" }).click();
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

  await page.getByLabel("File name").fill("id_first");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  await page.getByLabel("File name").fill("id_second");
  await page.getByRole("textbox", { name: "Passphrase" }).fill("end-to-end-passphrase");
  expect(await clickAndAwait(page, "Create key", "/api/v1/keys")).toBe(201);

  const before = await installation.read("id_first");
  const row = page.getByRole("row", { name: /id_first\b/ }).first();
  await row.getByRole("button", { name: "More actions" }).click();
  await row.getByRole("button", { name: "Rename or move" }).click();
  await page.getByLabel("Name", { exact: true }).fill("id_second");
  expect(await clickAndAwait(page, "Rename or move the key", "/api/v1/keys/")).toBe(409);

  await expect(page.getByRole("alert")).toContainText("already exists");
  expect(await installation.read("id_first")).toBe(before);
  expect(await installation.read("id_second.pub")).toContain("ssh-ed25519 ");
});

test("moves several chosen keys into a folder at once", async ({ page, installation }) => {
  await installation.write("conf.d/30-bulk.conf", "Host bulk\n\tIdentityFile ~/.ssh/id_bulk_a\n");
  await openApplication(page, installation);

  await openSection(page, "Groups");
  await page.getByLabel("New group name").fill("archive");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Keys");
  for (const name of ["id_bulk_a", "id_bulk_b"]) {
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
  await openSection(page, "Groups");
  await page.getByLabel("New group name").fill("archive");
  await page.getByRole("button", { name: "Add group" }).click();
  expect(await clickAndAwait(page, "Save groups", "/api/v1/config/save")).toBe(200);

  await openSection(page, "Keys");
  for (const name of ["id_kept", "id_filed"]) {
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

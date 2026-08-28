import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

const publicKey =
  "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl fixture@sshc";

test("shows the alias, effective user, fingerprint and the exact line before registering", async ({
  page,
  installation,
}) => {
  await openApplication(page, installation);
  await openSection(page, "Install Key on Server");
  await expect(page.getByRole("heading", { name: "Install Key on Server" })).toBeVisible();

  await expect(page.getByText("Nothing is sent to the remote host until you confirm it.")).toBeVisible();

  await page.getByLabel("Host alias").fill("bastion");
  await page.getByLabel("Public key file").fill("id_ed25519.pub");
  await page.getByLabel("Public key line").fill(publicKey);

  expect(await clickAndAwait(page, "Show what this would do", "/api/v1/remote-keys/plan")).toBe(200);

  const plan = page.getByRole("region", { name: "Confirm remote registration" });
  await expect(plan).toBeVisible();
  await expect(plan).toContainText("bastion");
  await expect(plan).toContainText("ops");
  await expect(plan).toContainText("203.0.113.10:2222");
  await expect(plan).toContainText("SHA256:");

  await expect(plan.locator('pre[aria-label="Public key line to append"]')).toContainText(
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl",
  );
  await expect(plan.locator('pre[aria-label="Remote command"]')).toContainText(
    "authorized_keys",
  );

  expect(await installation.read("config")).toContain("Host bastion");
});

test("refuses a public key that is not one valid line", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Install Key on Server");

  await page.getByLabel("Host alias").fill("bastion");
  await page.getByLabel("Public key file").fill("id_ed25519.pub");
  await page.getByLabel("Public key line").fill("echo pwned");

  expect(await clickAndAwait(page, "Show what this would do", "/api/v1/remote-keys/plan")).toBe(400);
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Confirm remote registration" }),
  ).toHaveCount(0);
});

test("filters and reviews multiple saved connections together", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Install Key on Server");

  const search = page.getByRole("searchbox", { name: "Host alias" });
  await search.fill("nas");
  await expect(page.getByRole("checkbox", { name: "nas" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "bastion" })).toHaveCount(0);
  await page.getByRole("checkbox", { name: "nas" }).check();
  await search.fill("bastion");
  await page.getByRole("checkbox", { name: "bastion" }).check();
  await expect(page.getByText("2 selected")).toBeVisible();

  await page.getByLabel("Public key file").fill("id_ed25519.pub");
  await page.getByLabel("Public key line").fill(publicKey);
  await page.getByRole("button", { name: "Show what this would do" }).click();

  const plan = page.getByRole("region", { name: "Confirm registration on 2 hosts" });
  await expect(plan).toBeVisible();
  await expect(plan.getByRole("article", { name: "Registration plan for bastion" })).toBeVisible();
  await expect(plan.getByRole("article", { name: "Registration plan for nas" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Register on 2 hosts" })).toBeEnabled();

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/remote-keys-multi-select.png`, fullPage: true });
    await plan.scrollIntoViewIfNeeded();
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/remote-keys-multi-plan.png`, fullPage: true });
  }
});

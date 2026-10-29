import { createHmac, randomBytes } from "node:crypto";
import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

// hashedHost writes host as OpenSSH's HashKnownHosts does: |1|salt|HMAC-SHA1(salt, host), both in base64.
function hashedHost(host: string): string {
  const salt = randomBytes(20);
  return `|1|${salt.toString("base64")}|${createHmac("sha1", salt).update(host).digest("base64")}`;
}

test("lists the known_hosts entries and deletes one through a confirmation", async ({
  page,
  installation,
}) => {
  const before = await installation.read("known_hosts");
  expect(before).toContain("203.0.113.10");

  await openApplication(page, installation);
  await openSection(page, "Known Hosts");
  await expect(page.getByRole("heading", { name: "Known Hosts" })).toBeVisible();

  const row = page.getByRole("row").filter({ hasText: "203.0.113.10" });
  await expect(row).toContainText("ssh-ed25519");
  await expect(row).toContainText("SHA256:");

  await row.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByText(/Remove line \d+/)).toBeVisible();

  expect(await clickAndAwait(page, "Confirm delete", "/api/v1/known-hosts/delete")).toBe(200);

  const after = await installation.read("known_hosts");
  expect(after).not.toContain("203.0.113.10");
});

test("keeps the search box scoped to the file it is showing", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Known Hosts");
  await expect(page.getByRole("row").filter({ hasText: "203.0.113.10" })).toBeVisible();

  await page.getByLabel("Search", { exact: true }).fill("no-such-host-anywhere");
  await expect(page.getByRole("row").filter({ hasText: "203.0.113.10" })).toHaveCount(0);

  expect(await installation.read("known_hosts")).toContain("203.0.113.10");
});

test("names the saved connection whose key a hashed entry holds", async ({ page, installation }) => {
  // nas is HostName 198.51.100.20 on the default port, so ssh looks its key up under that address.
  const key = "AAAAC3NzaC1lZDI1NTE5AAAAIGZpeHR1cmVrZXlmaXh0dXJla2V5Zml4dHVyZWtl";
  await installation.write("known_hosts", `${hashedHost("198.51.100.20")} ssh-ed25519 ${key}\n${hashedHost("192.0.2.99")} ssh-rsa ${key}\n`);

  await openApplication(page, installation);
  await openSection(page, "Known Hosts");
  await expect(page.getByRole("row").filter({ hasText: "ssh-ed25519" })).toContainText("nas (hashed)");
  const unknown = page.getByRole("row").filter({ hasText: "ssh-rsa" });
  await expect(unknown).toContainText("(hashed)");
  await expect(unknown).not.toContainText("nas");
});

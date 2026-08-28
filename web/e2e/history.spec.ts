import { clickAndAwait, expect, openSection, test, openApplication } from "./support/environment";

test("records a change in history and restores the previous bytes", async ({
  page,
  installation,
}) => {
  const original = await installation.read("config");

  await openApplication(page, installation);
  await openSection(page, "Connections");
  await page
    .getByRole("navigation", { name: "Connections" })
    .getByRole("button", { name: "bastion" })
    .click();
  await page.getByLabel("Port", { exact: true }).fill("2233");
  expect(await clickAndAwait(page, "Save Basic settings", "/api/v1/connections", "PATCH")).toBe(200);
  expect(await installation.read("config")).toContain("Port 2233");

  await openSection(page, "History");
  await expect(page.getByRole("heading", { name: "Completed changes" })).toBeVisible();
  await expect(page.getByText("Connection change", { exact: true })).toBeVisible();

  expect(await clickAndAwait(page, "Restore config", "/api/v1/history/restore")).toBe(200);

  expect(await installation.read("config")).toBe(original);
});

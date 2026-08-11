import { expect, openApplication, test } from "./support/environment";

test("starts with a searchable host launcher and contacts nothing unasked", async ({
  page,
  installation,
}) => {
  await installation.write(
    "sshc/metadata.json",
    JSON.stringify({
      schemaVersion: 1,
      hosts: [
        {
          identity: { path: "config", alias: "bastion" },
          tags: ["production"],
          favourite: true,
        },
      ],
    }),
  );

  const terminalRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith("/api/v1/terminal/")) terminalRequests.push(path);
  });

  await openApplication(page, installation);

  await expect(page.getByRole("heading", { name: "Your connections" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Home" })).toHaveAttribute("aria-current", "page");
  await expect(page.getByRole("list", { name: "Available connections" }).getByText(/bastion/)).toBeVisible();
  await expect(page.getByText("nas", { exact: true })).toBeVisible();
  expect(terminalRequests).toEqual([]);

  await page.getByRole("searchbox", { name: "Search connections" }).fill("production");
  await expect(page.getByRole("list", { name: "Available connections" }).getByText(/bastion/)).toBeVisible();
  await expect(page.getByText("nas", { exact: true })).toHaveCount(0);
  expect(terminalRequests).toEqual([]);

  await page.getByRole("button", { name: "Manage connections" }).click();
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();
});

test("opens the action menu without connecting, then keeps settings and connect distinct", async ({
  page,
  installation,
}) => {
  const launches: unknown[] = [];
  await page.route("**/api/v1/terminal/launch", async (route) => {
    launches.push(route.request().postDataJSON());
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ launched: true }),
    });
  });
  await openApplication(page, installation);

  await page.getByRole("button", { name: "Actions for bastion" }).click();
  expect(launches).toEqual([]);
  await page.getByRole("menuitem", { name: "Open connection settings" }).click();
  await expect(page).toHaveURL(/\/connections\/servers\?path=config&host=bastion&panel=basic$/);
  await expect(page.getByRole("tab", { name: "Basic" })).toHaveAttribute("aria-selected", "true");
  expect(launches).toEqual([]);

  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Home" }).click();
  await page.getByRole("button", { name: "Actions for bastion" }).click();
  await page.getByRole("menuitem", { name: "Connect", exact: true }).click();
  await expect.poll(() => launches).toEqual([{ alias: "bastion" }]);
});
